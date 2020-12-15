// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig"
	gyaml "github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/kubectl/pkg/scheme"
)

const (
	manifestURL  = "https://download.elastic.co/downloads/eck/%s/all-in-one.yaml"
	operatorName = "elastic-operator"

	csvTemplateFile     = "csv.tpl"
	packageTemplateFile = "package.tpl"

	crdFileSuffix     = "crd.yaml"
	csvFileSuffix     = "clusterserviceversion.yaml"
	packageFileSuffix = "package.yaml"
)

type cmdArgs struct {
	confPath     string
	allInOnePath string
	templatesDir string
}

var args = cmdArgs{}

func main() {
	cmd := &cobra.Command{
		Use:           "operatorhub",
		Short:         "Generate Operator Lifecycle Manager format files",
		Example:       "./operatorhub --conf=config.yaml",
		Version:       "0.2.0",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          doRun,
	}

	cmd.Flags().StringVar(&args.confPath, "conf", "", "Path to config file")
	_ = cmd.MarkFlagRequired("conf")

	cmd.Flags().StringVar(&args.allInOnePath, "all-in-one", "", "Path to all-in-one.yaml")
	cmd.Flags().StringVar(&args.templatesDir, "templates", "./templates", "Path to the templates directory")

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

	allInOneStream, err := getAllInOneStream(conf, args.allInOnePath)
	if err != nil {
		return err
	}

	defer allInOneStream.Close()

	extracts, err := extractYAMLParts(allInOneStream)
	if err != nil {
		return err
	}

	for i := range conf.Packages {
		params, err := buildRenderParams(conf, i, extracts)
		if err != nil {
			return err
		}

		outDir := conf.Packages[i].OutputPath
		if err := render(params, args.templatesDir, outDir); err != nil {
			return err
		}
	}

	return nil
}

type config struct {
	NewVersion   string `json:"newVersion"`
	PrevVersion  string `json:"prevVersion"`
	StackVersion string `json:"stackVersion"`
	CRDs         []struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
	} `json:"crds"`
	Packages []struct {
		OutputPath          string `json:"outputPath"`
		PackageName         string `json:"packageName"`
		DistributionChannel string `json:"distributionChannel"`
		OperatorRepo        string `json:"operatorRepo"`
		UbiOnly             bool   `json:"ubiOnly"`
	} `json:"packages"`
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

func getAllInOneStream(conf *config, allInOnePath string) (io.ReadCloser, error) {
	if allInOnePath == "" {
		return allInOneFromWeb(conf.NewVersion)
	}

	f, err := os.Open(allInOnePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", allInOnePath, err)
	}

	return f, nil
}

func allInOneFromWeb(version string) (io.ReadCloser, error) {
	url := fmt.Sprintf(manifestURL, version)

	client := http.Client{}

	ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFunc()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to %s: %w", url, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to GET %s: %w", url, err)
	}

	defer func() {
		if resp.Body != nil {
			resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request error: %s", resp.Status)
	}

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, resp.Body)

	return ioutil.NopCloser(buf), err
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

func extractYAMLParts(stream io.Reader) (*yamlExtracts, error) {
	if err := apiextv1beta1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("failed to register api-extensions: %w", err)
	}

	decoder := scheme.Codecs.UniversalDeserializer()
	yamlReader := yaml.NewYAMLReader(bufio.NewReader(stream))

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
	NewVersion     string
	ShortVersion   string
	PrevVersion    string
	StackVersion   string
	OperatorRepo   string
	OperatorRBAC   string
	AdditionalArgs []string
	CRDList        []*CRD
	PackageName    string
}

func buildRenderParams(conf *config, packageIndex int, extracts *yamlExtracts) (*RenderParams, error) {
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
		return nil, fmt.Errorf("failed to marshal operator RBAC rules: %w", err)
	}

	var additionalArgs []string
	if conf.Packages[packageIndex].UbiOnly {
		additionalArgs = append(additionalArgs, "--ubi-only")
	}

	additionalArgs = append(additionalArgs, "--distribution-channel="+conf.Packages[packageIndex].DistributionChannel)

	return &RenderParams{
		NewVersion:     conf.NewVersion,
		ShortVersion:   strings.Join(versionParts[:2], "."),
		PrevVersion:    conf.PrevVersion,
		StackVersion:   conf.StackVersion,
		OperatorRepo:   conf.Packages[packageIndex].OperatorRepo,
		AdditionalArgs: additionalArgs,
		CRDList:        crdList,
		OperatorRBAC:   string(rbac),
		PackageName:    conf.Packages[packageIndex].PackageName,
	}, nil
}

func render(params *RenderParams, templatesDir, outDir string) error {
	versionDir := filepath.Join(outDir, params.NewVersion)

	// if the directory exists, throw an error because overwriting/merging is dangerous
	_, err := os.Stat(versionDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to stat %s: %w", versionDir, err)
		}
	} else {
		return fmt.Errorf("directory already exists: %s", versionDir)
	}

	if err := os.MkdirAll(versionDir, 0o766); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", versionDir, err)
	}

	if err := renderCSVFile(params, templatesDir, versionDir); err != nil {
		return err
	}

	if err := renderCRDs(params, versionDir); err != nil {
		return err
	}

	// package file is written outside the version directory
	return renderPackageFile(params, templatesDir, outDir)
}

func renderCSVFile(params *RenderParams, templatesDir, outDir string) error {
	templateFile := filepath.Join(templatesDir, csvTemplateFile)
	csvFile := filepath.Join(outDir, fmt.Sprintf("%s.v%s.%s", params.PackageName, params.NewVersion, csvFileSuffix))

	return renderTemplate(params, templateFile, csvFile)
}

func renderPackageFile(params *RenderParams, templatesDir, outDir string) error {
	templateFile := filepath.Join(templatesDir, packageTemplateFile)
	pkgFile := filepath.Join(outDir, fmt.Sprintf("%s.%s", params.PackageName, packageFileSuffix))

	return renderTemplate(params, templateFile, pkgFile)
}

func renderTemplate(params *RenderParams, templatePath, outPath string) error {
	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(sprig.TxtFuncMap()).ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("failed to parse template at %s: %w", templatePath, err)
	}

	outFile, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open file for writing [%s]: %w", outPath, err)
	}

	defer outFile.Close()

	return tmpl.Execute(outFile, params)
}

func renderCRDs(params *RenderParams, outDir string) error {
	for _, crd := range params.CRDList {
		crdFileName := fmt.Sprintf("%s.%s", strings.ToLower(crd.Name), crdFileSuffix)
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
