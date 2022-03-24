// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

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

	"github.com/Masterminds/sprig/v3"
	gyaml "github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/kubectl/pkg/scheme"
)

const (
	allInOneURL         = "https://download.elastic.co/downloads/eck/%s/all-in-one.yaml"
	crdManifestURL      = "https://download.elastic.co/downloads/eck/%s/crds.yaml"
	operatorManifestURL = "https://download.elastic.co/downloads/eck/%s/operator.yaml"
	operatorName        = "elastic-operator"

	csvTemplateFile     = "csv.tpl"
	packageTemplateFile = "package.tpl"

	crdFileSuffix     = "crd.yaml"
	csvFileSuffix     = "clusterserviceversion.yaml"
	packageFileSuffix = "package.yaml"

	yamlSeparator = "---\n"
)

type cmdArgs struct {
	confPath      string
	manifestPaths []string
	templatesDir  string
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

	cmd.Flags().StringVar(&args.confPath, "conf", "config.yaml", "Path to config file")

	cmd.Flags().StringSliceVar(&args.manifestPaths, "yaml-manifest", nil, "Path to installation manifests")
	cmd.Flags().StringVar(&args.templatesDir, "templates", "./templates", "Path to the templates directory")

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func doRun(_ *cobra.Command, _ []string) error {
	conf, err := loadConfig(args.confPath)
	if err != nil {
		return fmt.Errorf("when loading config: %w", err)
	}

	manifestStream, close, err := getInstallManifestStream(conf, args.manifestPaths)
	if err != nil {
		return fmt.Errorf("when getting install manifest stream: %w", err)
	}

	defer close()

	extracts, err := extractYAMLParts(manifestStream)
	if err != nil {
		return fmt.Errorf("when extracting YAML parts: %w", err)
	}

	for i := range conf.Packages {
		params, err := buildRenderParams(conf, i, extracts)
		if err != nil {
			return fmt.Errorf("when building render params: %w", err)
		}

		outDir := conf.Packages[i].OutputPath
		if err := render(params, args.templatesDir, outDir); err != nil {
			return fmt.Errorf("when rendering: %w", err)
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

var errNotFound = errors.New("not found")

func getInstallManifestStream(conf *config, manifestPaths []string) (io.Reader, func(), error) {
	if len(manifestPaths) == 0 {
		s, err := installManifestFromWeb(conf.NewVersion)
		return s, func() {}, err
	}

	var rs []io.Reader
	closer := func() {
		for _, r := range rs {
			if closer, ok := r.(io.Closer); ok {
				closer.Close()
			}
		}
	}
	for _, p := range manifestPaths {
		r, err := os.Open(p)
		if err != nil {
			return nil, closer, fmt.Errorf("failed to open %s: %w", manifestPaths, err)
		}
		rs = append(rs, r)
		// if we're using local yaml files, ensure that they have a proper
		// end of directives marker between them.
		rs = append(rs, strings.NewReader(yamlSeparator))
	}
	return io.MultiReader(rs...), closer, nil
}

func installManifestFromWeb(version string) (io.Reader, error) {
	// try the legacy all-in-one first for older releases
	buf, err := makeRequest(fmt.Sprintf(allInOneURL, version))
	if err == errNotFound {
		// if not found load the separate manifests for CRDs and operator (version >= 1.7.0)
		crdManifestURL := fmt.Sprintf(crdManifestURL, version)
		crds, err := makeRequest(crdManifestURL)
		if err != nil {
			return nil, fmt.Errorf("when getting %s: %w", crdManifestURL, err)
		}
		operatorManifestURL := fmt.Sprintf(operatorManifestURL, version)
		op, err := makeRequest(operatorManifestURL)
		if err != nil {
			return nil, fmt.Errorf("when getting %s: %w", operatorManifestURL, err)
		}
		return io.MultiReader(crds, strings.NewReader(yamlSeparator), op), nil
	}
	return buf, err
}

func makeRequest(url string) (io.Reader, error) {
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

	if resp.StatusCode == http.StatusNotFound {
		return nil, errNotFound
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request error %s: %s", url, resp.Status)
	}

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, resp.Body)

	return buf, err
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

// WebhookDefinition corresponds to a WebhookDefinition within an OLM
// ClusterServiceVersion.
// See https://olm.operatorframework.io/docs/advanced-tasks/adding-admission-and-conversion-webhooks/
type WebhookDefinition struct {
	AdmissionReviewVersions []string                         `json:"admissionReviewVersions"`
	ContainerPort           int                              `json:"containerPort"`
	DeploymentName          string                           `json:"deploymentName"`
	FailurePolicy           *admissionv1.FailurePolicyType   `json:"failurePolicy"`
	MatchPolicy             admissionv1.MatchPolicyType      `json:"matchPolicy"`
	GenerateName            string                           `json:"generateName"`
	Rules                   []admissionv1.RuleWithOperations `json:"rules"`
	SideEffects             *admissionv1.SideEffectClass     `json:"sideEffects"`
	TargetPort              int                              `json:"targetPort"`
	Type                    string                           `json:"type"`
	WebhookPath             *string                          `json:"webhookPath"`
}

type yamlExtracts struct {
	crds             map[string]*CRD
	operatorRBAC     []rbacv1.PolicyRule
	operatorWebhooks []admissionv1.ValidatingWebhookConfiguration
}

func extractYAMLParts(stream io.Reader) (*yamlExtracts, error) {
	if err := apiextv1beta1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("failed to register apiextensions/v1beta1: %w", err)
	}

	if err := apiextv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("failed to register apiextensions/v1: %w", err)
	}

	if err := admissionv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("failed to register admissionregistration/v1: %w", err)
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

		yamlBytes = normalizeTrailingNewlines(yamlBytes)

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
		case *apiextv1.CustomResourceDefinition:
			parts.crds[obj.Name] = &CRD{
				Name:    obj.Name,
				Group:   obj.Spec.Group,
				Kind:    obj.Spec.Names.Kind,
				Version: obj.Spec.Versions[0].Name,
				Def:     yamlBytes,
			}
		case *rbacv1.ClusterRole:
			if obj.Name == operatorName {
				parts.operatorRBAC = obj.Rules
			}
		case *admissionv1.ValidatingWebhookConfiguration:
			parts.operatorWebhooks = append(parts.operatorWebhooks, *obj)
		}
	}
}

// normalizeTrailingNewlines removed duplicate newlines at the end of the documents to satisfy YAML linter rules.
func normalizeTrailingNewlines(yamlBytes []byte) []byte {
	trimmed := bytes.TrimRight(yamlBytes, "\n")
	return append(trimmed, "\n"...)
}

type RenderParams struct {
	NewVersion       string
	ShortVersion     string
	PrevVersion      string
	StackVersion     string
	OperatorRepo     string
	OperatorRBAC     string
	AdditionalArgs   []string
	CRDList          []*CRD
	OperatorWebhooks string
	PackageName      string
	UbiOnly          bool
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

	var webhookDefinitionList []WebhookDefinition

	for _, webhook := range extracts.operatorWebhooks {
		webhookDefinitionList = append(webhookDefinitionList, validatingWebhookConfigurationToWebhookDefinition(webhook)...)
	}

	webhooks, err := gyaml.Marshal(webhookDefinitionList)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal operator webhook rules: %w", err)
	}

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
		NewVersion:       conf.NewVersion,
		ShortVersion:     strings.Join(versionParts[:2], "."),
		PrevVersion:      conf.PrevVersion,
		StackVersion:     conf.StackVersion,
		OperatorRepo:     conf.Packages[packageIndex].OperatorRepo,
		AdditionalArgs:   additionalArgs,
		CRDList:          crdList,
		OperatorWebhooks: string(webhooks),
		OperatorRBAC:     string(rbac),
		PackageName:      conf.Packages[packageIndex].PackageName,
		UbiOnly:          conf.Packages[packageIndex].UbiOnly,
	}, nil
}

// validatingWebhookConfigurationToWebhookDefinition converts a standard validating webhook configuration resource
// to an OLM webhook definition resource.
func validatingWebhookConfigurationToWebhookDefinition(webhookConfiguration admissionv1.ValidatingWebhookConfiguration) []WebhookDefinition {
	var webhookDefinitions []WebhookDefinition
	for _, webhook := range webhookConfiguration.Webhooks {
		webhookDefinitions = append(webhookDefinitions, WebhookDefinition{
			Type:                    "ValidatingAdmissionWebhook",
			AdmissionReviewVersions: webhook.AdmissionReviewVersions,
			TargetPort:              9443,
			ContainerPort:           443,
			DeploymentName:          "elastic-operator",
			FailurePolicy:           webhook.FailurePolicy,
			MatchPolicy:             admissionv1.Exact,
			GenerateName:            webhook.Name,
			Rules:                   webhook.Rules,
			SideEffects:             webhook.SideEffects,
			WebhookPath:             webhook.ClientConfig.Service.Path,
		})
	}
	return webhookDefinitions
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
		err := os.RemoveAll(versionDir)
		if err != nil {
			return fmt.Errorf("failed to remove existing directory: %w", err)
		}
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
