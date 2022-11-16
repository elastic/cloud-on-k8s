// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package internal

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// GenerateFlags holds flag values for the generate operation.
type GenerateFlags struct {
	Source            string
	Profile           string
	Values            []string
	ValueFiles        []string
	OperatorNamespace string
}

// OptionsFlags holds flag values for the options operation.
type OptionsFlags struct {
	Source string
}

// Generate produces a manifest for installing ECK using an embedded Helm chart.
func Generate(opts *GenerateFlags) error {
	chartPath, err := filepath.Abs(opts.Source)
	if err != nil {
		return err
	}

	settings := cli.New()
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(settings.RESTClientGetter(), opts.OperatorNamespace, os.Getenv("HELM_DRIVER"), log.Printf); err != nil {
		return err
	}

	valueFiles := []string{}
	if opts.Profile != "" {
		valueFiles = append(valueFiles, filepath.Join(chartPath, fmt.Sprintf("profile-%s.yaml", opts.Profile)))
	}
	valueFiles = append(valueFiles, opts.ValueFiles...)

	// set manifestGen flag
	valueFlags := append(opts.Values, "global.manifestGen=true")

	valueOpts := &values.Options{
		Values:     valueFlags,
		ValueFiles: valueFiles,
	}

	client := action.NewInstall(actionConfig)
	client.DryRun = true
	client.Replace = true
	client.ClientOnly = true
	client.IncludeCRDs = false
	client.Version = ">0.0.0-0"
	client.ReleaseName = "elastic-operator"
	client.Namespace = opts.OperatorNamespace
	client.PostRenderer = helmLabelRemover{}
	// Arbitrarily sets a k8s version greater than the min required version in the Chart, otherwise v1.20 is used by default
	// because Helm doesn't connect to a real K8S API server (clientOnly = true).
	fakeKubeVersion, err := chartutil.ParseKubeVersion("v9.99.99")
	if err != nil {
		return fmt.Errorf("invalid fake kube version %q: %s", fakeKubeVersion, err)
	}
	client.KubeVersion = fakeKubeVersion

	vals, err := valueOpts.MergeValues(getter.All(settings))
	if err != nil {
		return err
	}

	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		return err
	}

	rel, err := client.Run(chartRequested, vals)
	if err != nil {
		return err
	}

	if rel != nil {
		var manifests bytes.Buffer

		fmt.Fprintln(&manifests, strings.TrimSpace(rel.Manifest))

		for _, m := range rel.Hooks {
			fmt.Fprintf(&manifests, "---\n# Source: %s\n%s\n", m.Path, m.Manifest)
		}

		fmt.Println(manifests.String())
	}

	return nil
}

// Options lists the available chart options that can be set.
func Options(opts *OptionsFlags) error {
	chartPath, err := filepath.Abs(opts.Source)
	if err != nil {
		return err
	}

	fmt.Println(chartPath)

	client := action.NewShow(action.ShowValues)
	client.Version = ">0.0.0-0"

	out, err := client.Run(chartPath)
	if err != nil {
		return err
	}

	fmt.Println(out)

	return nil
}

// helmLabelRemover implements the PostRenderer interface. It is used to remove Helm-specific labels from the generated manifests.
type helmLabelRemover struct{}

func (hlr helmLabelRemover) Run(renderedManifests *bytes.Buffer) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)

	pipeline := kio.Pipeline{
		Inputs:  []kio.Reader{&kio.ByteReader{Reader: renderedManifests}},
		Filters: []kio.Filter{kio.FilterFunc(hlr.removeHelmLabels)},
		Outputs: []kio.Writer{kio.ByteWriter{Writer: buf}},
	}

	if err := pipeline.Execute(); err != nil {
		return nil, err
	}

	return buf, nil
}

func (hlr helmLabelRemover) removeHelmLabels(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	for i := range nodes {
		n := nodes[i]
		for _, label := range []string{"helm.sh/chart", "app.kubernetes.io/managed-by"} {
			if err := n.PipeE(yaml.Get("metadata"), yaml.Get("labels"), yaml.Clear(label)); err != nil {
				return nil, fmt.Errorf("failed to remove label %s: %w", label, err)
			}
		}
	}

	return nodes, nil
}
