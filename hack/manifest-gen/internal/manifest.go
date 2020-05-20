// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

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
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
)

// GenerateFlags holds flag values for the generate operation.
type GenerateFlags struct {
	Source      string
	Profile     string
	Values      []string
	ValueFiles  []string
	ExcludeCRDs bool
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

	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), os.Getenv("HELM_DRIVER"), log.Printf); err != nil {
		return err
	}

	valueFiles := []string{filepath.Join(chartPath, fmt.Sprintf("profile-%s.yaml", opts.Profile))}
	valueFiles = append(valueFiles, opts.ValueFiles...)

	valueOpts := &values.Options{
		Values:     opts.Values,
		ValueFiles: valueFiles,
	}

	client := action.NewInstall(actionConfig)
	client.DryRun = true
	client.Replace = true
	client.ClientOnly = true
	client.IncludeCRDs = !opts.ExcludeCRDs
	client.Version = ">0.0.0-0"
	client.ReleaseName = "eck"
	client.Namespace = settings.Namespace()

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
