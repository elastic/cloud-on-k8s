package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
	gyaml "github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/kubectl/pkg/scheme"
)

const (
	operatorName = "elastic-operator"
)

type cmdArgs struct {
	confPath     string
	allInOnePath string
	templatePath string
	outPath      string
}

var args = cmdArgs{}

func main() {
	cmd := &cobra.Command{
		Use:           "operatorhub",
		Short:         "Generate Operator Hub CSV and other files",
		Example:       "./operatorhub --conf=example/config.yaml --out=$TMP/eck",
		Version:       "0.1.0",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          doRun,
	}

	cmd.Flags().StringVar(&args.confPath, "conf", "", "Path to config file")
	_ = cmd.MarkFlagRequired("conf")

	cmd.Flags().StringVar(&args.outPath, "out", "", "Path to output the artefacts")
	_ = cmd.MarkFlagRequired("out")

	cmd.Flags().StringVar(&args.allInOnePath, "all-in-one", "../../config/all-in-one.yaml", "Path to all-in-one.yaml")
	cmd.Flags().StringVar(&args.templatePath, "template", "template/csv.tpl", "Path to the CSV template")

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func doRun(_ *cobra.Command, _ []string) error {
	conf, err := loadConfig(args.confPath)
	if err != nil {
		return err
	}

	extracts, err := extractYAMLParts(args.allInOnePath)
	if err != nil {
		return err
	}

	params, err := buildRenderParams(conf, extracts)
	if err != nil {
		return err
	}

	return render(params, args.templatePath, args.outPath)
}

type config struct {
	NewVersion    string `json:"newVersion"`
	PrevVersion   string `json:"prevVersion"`
	StackVersion  string `json:"stackVersion"`
	OperatorImage string `json:"operatorImage"`
	CRDs          []struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
	} `json:"crds"`
}

func loadConfig(path string) (*config, error) {
	confBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", path, err)
	}

	var conf config
	if err := gyaml.Unmarshal(confBytes, &conf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config from %s: %w", path, err)
	}

	return &conf, nil
}

type CRD struct {
	Name        string
	Group       string
	Kind        string
	Version     string
	DisplayName string
	Description string
	Def         []byte
}

type yamlExtracts struct {
	crds         map[string]*CRD
	operatorRBAC []rbacv1.PolicyRule
}

func extractYAMLParts(path string) (*yamlExtracts, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", path, err)
	}

	defer f.Close()

	if err := apiextv1beta1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("failed to register api-extensions: %w", err)
	}

	decoder := scheme.Codecs.UniversalDeserializer()
	yamlReader := yaml.NewYAMLReader(bufio.NewReader(f))

	parts := &yamlExtracts{
		crds: make(map[string]*CRD),
	}

	for {
		yamlBytes, err := yamlReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return parts, nil
			}

			return nil, fmt.Errorf("failed to read CRD YAML: %w", err)
		}

		runtimeObj, _, err := decoder.Decode(yamlBytes, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to decode CRD YAML: %w", err)
		}

		switch obj := runtimeObj.(type) {
		case *apiextv1beta1.CustomResourceDefinition:
			parts.crds[obj.Name] = &CRD{
				Name:    obj.Name,
				Group:   obj.Spec.Group,
				Kind:    obj.Spec.Names.Kind,
				Version: obj.Spec.Version,
				Def:     yamlBytes,
			}
		case *rbacv1.ClusterRole:
			if obj.Name == operatorName {
				parts.operatorRBAC = obj.Rules
			}
		}
	}
}

type RenderParams struct {
	NewVersion    string
	ShortVersion  string
	PrevVersion   string
	StackVersion  string
	OperatorImage string
	OperatorRBAC  string
	CRDList       []*CRD
}

func buildRenderParams(conf *config, extracts *yamlExtracts) (*RenderParams, error) {
	for _, c := range conf.CRDs {
		if crd, ok := extracts.crds[c.Name]; ok {
			crd.DisplayName = c.DisplayName
			crd.Description = c.Description
		}
	}

	crdList := make([]*CRD, 0, len(extracts.crds))

	var missing []string
	for _, crd := range extracts.crds {
		if strings.TrimSpace(crd.Description) == "" || strings.TrimSpace(crd.DisplayName) == "" {
			missing = append(missing, crd.Name)
		}

		crdList = append(crdList, crd)
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("config file does not contain descriptions for some CRDs: %+v", missing)
	}

	sort.Slice(crdList, func(i, j int) bool {
		return crdList[i].Name <= crdList[j].Name
	})

	versionParts := strings.Split(conf.NewVersion, ".")
	if len(versionParts) < 2 {
		return nil, fmt.Errorf("newVersion in config file appears to be invalid [%s]", conf.NewVersion)
	}

	rbac, err := gyaml.Marshal(extracts.operatorRBAC)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal operator RBCA rules: %w", err)
	}

	return &RenderParams{
		NewVersion:    conf.NewVersion,
		ShortVersion:  strings.Join(versionParts[:2], "."),
		PrevVersion:   conf.PrevVersion,
		StackVersion:  conf.StackVersion,
		OperatorImage: conf.OperatorImage,
		CRDList:       crdList,
		OperatorRBAC:  string(rbac),
	}, nil
}

func render(params *RenderParams, templatePath, outPath string) error {
	outDir := filepath.Join(outPath, params.NewVersion)
	if err := os.MkdirAll(outDir, 0766); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", outDir, err)
	}

	if err := renderCSV(params, templatePath, outDir); err != nil {
		return err
	}

	return renderCRDs(params, outDir)
}

func renderCSV(params *RenderParams, templatePath, outDir string) error {
	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(sprig.TxtFuncMap()).ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("failed to parse template at %s: %w", templatePath, err)
	}

	csvFileName := fmt.Sprintf("elastic-cloud-eck.v%s.clusterserviceversion.yaml", params.NewVersion)
	csvPath := filepath.Join(outDir, csvFileName)

	csvFile, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", csvPath, err)
	}

	defer csvFile.Close()

	return tmpl.Execute(csvFile, params)
}

func renderCRDs(params *RenderParams, outDir string) error {
	for _, crd := range params.CRDList {
		crdFileName := fmt.Sprintf("%s.crd.yaml", strings.ToLower(crd.Name))
		crdPath := filepath.Join(outDir, crdFileName)

		crdFile, err := os.Create(crdPath)
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", crdPath, err)
		}

		if err := writeCRD(crdFile, crd.Def); err != nil {
			return fmt.Errorf("failed to write to %s: %w", crdPath, err)
		}
	}

	return nil
}

func writeCRD(out io.WriteCloser, data []byte) error {
	defer out.Close()

	_, err := io.Copy(out, bytes.NewReader(data))

	return err
}
