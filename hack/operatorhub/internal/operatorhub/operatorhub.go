// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package operatorhub

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/flags"
	gyaml "github.com/ghodss/yaml"
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

	imagesEndpoint = "https://catalog.redhat.com/api/containers/v1/projects/certification/id/%s/images"

	httpAcceptHeader               = "Accept"
	httpContentTypeHeader          = "Content-Type"
	httpXAPIKeyHeader              = "X-API-KEY"
	httpApplicationJSONHeaderValue = "application/json"
)

var (
	errNotFound = errors.New("not found")
)

// GenerateConfig is the configuration for the generate operation
type GenerateConfig struct {
	ConfigFile      *flags.Config
	ManifestPaths   []string
	TemplatesPath   string
	RedhatAPIKey    string
	RedhatProjectID string
}

// Generate will generate the Operator Lifecycle Manager format files
func Generate(config GenerateConfig) error {
	var imageDigest string
	if config.ConfigFile.HasDigestPinning() {
		if len(config.RedhatAPIKey) == 0 {
			return errors.New("Red Hat API key is required to get image digest")
		}
		if len(config.RedhatProjectID) == 0 {
			return errors.New("Red Hat project ID is required to get image digest")
		}
		var err error
		imageDigest, err = getImageDigest(config.RedhatAPIKey, config.RedhatProjectID, config.ConfigFile.NewVersion)
		if err != nil {
			log.Println("ⅹ")
			return err
		}
	}

	log.Printf("Gathering and extracting data from yaml manifest path %v ", config.ManifestPaths)
	manifestStream, close, err := getInstallManifestStream(config.ConfigFile, config.ManifestPaths)
	if err != nil {
		log.Println("ⅹ")
		return err
	}
	defer close()
	extracts, err := extractYAMLParts(manifestStream)
	if err != nil {
		log.Println("ⅹ")
		return err
	}
	if len(extracts.crds) == 0 {
		log.Println("ⅹ")
		return errors.New("no crds found")
	}
	if len(extracts.operatorRBAC) == 0 {
		log.Println("ⅹ")
		return errors.New("no operator RBAC found")
	}
	if len(extracts.operatorWebhooks) == 0 {
		log.Println("ⅹ")
		return errors.New("no operator webhooks found")
	}
	log.Println("✓")

	log.Printf("Rendering final operatorhub data ")
	for i := range config.ConfigFile.Packages {
		params, err := buildRenderParams(config.ConfigFile, i, extracts, imageDigest)
		if err != nil {
			log.Println("ⅹ")
			return err
		}

		outDir := config.ConfigFile.Packages[i].OutputPath
		if err := render(params, config.TemplatesPath, outDir); err != nil {
			log.Println("ⅹ")
			return err
		}
	}

	log.Println("✓")
	return nil
}

type Images struct {
	Data []struct {
		DockerImageDigest string `json:"docker_image_digest"`
		Id                string `json:"_id"`
		CreationDate      string `json:"creation_date"`
	} `json:"data"`
}

// getImageDigest connects to the Red Hat certification API to get the certified
// operator image digest as it is exposed by the Red Hat registry.
func getImageDigest(apiKey, projectId, version string) (string, error) {
	requestURL := fmt.Sprintf(imagesEndpoint, projectId)

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set(httpContentTypeHeader, httpApplicationJSONHeaderValue)
	req.Header.Set(httpAcceptHeader, httpApplicationJSONHeaderValue)
	req.Header.Set(httpXAPIKeyHeader, apiKey)

	q := req.URL.Query()
	q.Add("filter", fmt.Sprintf("repositories.tags.name==%s;deleted==false", version))
	req.URL.RawQuery = q.Encode()

	client := http.Client{
		Timeout: 30 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request error %s: %s", requestURL, res.Status)
	}

	var images Images
	if err := json.NewDecoder(res.Body).Decode(&images); err != nil {
		return "", err
	}
	if len(images.Data) > 1 {
		fmt.Fprintf(os.Stderr, "\nid                       creation_date                    docker_image_digest\n")
		for _, image := range images.Data {
			fmt.Fprintf(os.Stderr, "%s %s %s\n", image.Id, image.CreationDate, image.DockerImageDigest)
		}
		return "", fmt.Errorf("found %d images with tag %s in RedHat catalog while only one is expected", len(images.Data), version)
	}
	if len(images.Data) == 0 {
		return "", fmt.Errorf("no image with tag %s in RedHat catalog", version)
	}
	imageDigest := images.Data[0].DockerImageDigest
	if imageDigest == "" {
		return "", fmt.Errorf("image digest for %s is empty", version)
	}
	return imageDigest, nil
}

func getInstallManifestStream(conf *flags.Config, manifestPaths []string) (io.Reader, func(), error) {
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
			return nil, closer, fmt.Errorf("while opening %s: %w", manifestPaths, err)
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
		crds, err := makeRequest(fmt.Sprintf(crdManifestURL, version))
		if err != nil {
			return nil, err
		}
		op, err := makeRequest(fmt.Sprintf(operatorManifestURL, version))
		if err != nil {
			return nil, err
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
		return nil, fmt.Errorf("while creating request to %s: %w", url, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("while executing HTTP GET %s: %w", url, err)
	}
	defer resp.Body.Close()

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

// CRD defines a custom resource definition
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
		return nil, fmt.Errorf("while registering apiextensions/v1beta1: %w", err)
	}

	if err := apiextv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("while registering apiextensions/v1: %w", err)
	}

	if err := admissionv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("while registering admissionregistration/v1: %w", err)
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

			return nil, fmt.Errorf("while reading CRD YAML: %w", err)
		}

		yamlBytes = normalizeTrailingNewlines(yamlBytes)

		runtimeObj, _, err := decoder.Decode(yamlBytes, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("while decoding CRD YAML: %w", err)
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

// RenderParams are the parameters sent to the render operation
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
	Tag              string
	UbiOnly          bool
}

func buildRenderParams(conf *flags.Config, packageIndex int, extracts *yamlExtracts, imageDigest string) (*RenderParams, error) {
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
		return nil, fmt.Errorf("while marshaling operator webhook rules: %w", err)
	}

	versionParts := strings.Split(conf.NewVersion, ".")
	if len(versionParts) < 2 {
		return nil, fmt.Errorf("newVersion in config file appears to be invalid [%s]", conf.NewVersion)
	}

	rbac, err := gyaml.Marshal(extracts.operatorRBAC)
	if err != nil {
		return nil, fmt.Errorf("while marshaling operator RBAC rules: %w", err)
	}

	var additionalArgs []string
	if conf.Packages[packageIndex].UbiOnly {
		additionalArgs = append(additionalArgs, "--ubi-only")
	}

	additionalArgs = append(additionalArgs, "--distribution-channel="+conf.Packages[packageIndex].DistributionChannel)

	tag := ":" + conf.NewVersion
	if conf.Packages[packageIndex].DigestPinning {
		tag = "@" + imageDigest
	}

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
		Tag:              tag,
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
			return fmt.Errorf("while removing existing directory: %w", err)
		}
	}

	if err := os.MkdirAll(versionDir, 0o766); err != nil {
		return fmt.Errorf("while creating directory %s: %w", versionDir, err)
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
	if strings.HasPrefix(params.NewVersion, "v") {
		csvFile = filepath.Join(outDir, fmt.Sprintf("%s.%s.%s", params.PackageName, params.NewVersion, csvFileSuffix))
	}

	return renderTemplate(params, templateFile, csvFile)
}

func renderPackageFile(params *RenderParams, templatesDir, outDir string) error {
	templateFile := filepath.Join(templatesDir, packageTemplateFile)
	pkgFile := filepath.Join(outDir, fmt.Sprintf("%s.%s", params.PackageName, packageFileSuffix))

	return renderTemplate(params, templateFile, pkgFile)
}

func renderTemplate(params *RenderParams, templatePath, outPath string) error {
	// ensure NewVersion is never prefixed with 'v' when rendering template
	// as we use the 'v' prefix in the `name:` field, but the `version:` field
	// cannnot have the 'v' prefix, as the certified operator automation
	// refused to accept this field with a 'v' prefix.
	params.NewVersion = strings.TrimPrefix(params.NewVersion, "v")

	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(sprig.TxtFuncMap()).ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("while parsing template at %s: %w", templatePath, err)
	}

	outFile, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("while opening file for writing [%s]: %w", outPath, err)
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
			return fmt.Errorf("while creating %s: %w", crdPath, err)
		}

		if err := writeCRD(crdFile, crd.Def); err != nil {
			return fmt.Errorf("while writing to %s: %w", crdPath, err)
		}
	}

	return nil
}

func writeCRD(out io.WriteCloser, data []byte) error {
	defer out.Close()

	_, err := io.Copy(out, bytes.NewReader(data))

	return err
}
