// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manifests

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/flags"
	hub "github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/pkg/operatorhub"
)

// Command will return the generate-manifests command
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate-manifests",
		Short: "Generate operator lifecycle manager format files",
		Long: `Generate operator lifecycle manager format files within 
'community-operators', 'certified-operators', and 'upstream-community-operators' directories.`,
		Example:      "operatorhub generate-manifests -c ./config.yaml -n 2.6.0 -p 2.5.0 -s 8.6.0",
		SilenceUsage: true,
		PreRunE:      preRunE,
		RunE:         doRun,
	}

	cmd.Flags().StringVarP(
		&flags.PreviousVersion,
		flags.PrevVersionFlag,
		"V",
		"",
		"Previous version of the operator to populate 'replaces' in operator cluster service version yaml (PREV_VERSION)",
	)

	cmd.Flags().StringVarP(
		&flags.StackVersion,
		flags.StackVersionFlag,
		"s",
		"",
		"Stack version of Elastic stack used to populate the operator cluster service version yaml (STACK_VERSION)",
	)

	cmd.Flags().StringVarP(
		&flags.Conf,
		flags.ConfFlag,
		"c",
		"./config.yaml",
		"Path to config file to read CRDs, and packages (CONF)",
	)

	cmd.Flags().StringSliceVarP(
		&flags.YamlManifest,
		flags.YamlManifestFlag,
		"y",
		nil,
		"Path to installation manifests (YAML_MANIFEST)",
	)

	cmd.Flags().StringVarP(
		&flags.Templates,
		flags.TemplatesFlag,
		"T",
		"./templates",
		"Path to the templates directory (TEMPLATES)",
	)

	return cmd
}

// preRunE are the pre-run operations for the generate-manifests command
func preRunE(cmd *cobra.Command, args []string) error {
	if flags.Conf == "" {
		return fmt.Errorf("%s is required", flags.ConfFlag)
	}

	if flags.Tag == "" {
		return fmt.Errorf("%s is required", flags.TagFlag)
	}

	// If no yaml manifests are given, then the stack, and previous version
	// flags are required to generate manifests.
	if len(flags.YamlManifest) == 0 {
		if flags.PreviousVersion == "" {
			return fmt.Errorf("%s is required", flags.PrevVersionFlag)
		}

		if flags.StackVersion == "" {
			return fmt.Errorf("%s is required", flags.StackVersionFlag)
		}
	}

	// validation for apikey, and redhat project id is not done
	// here, but is done after reading the configuration file
	// and checking whether digest pinning `digestPinning: true`
	// is enabled for any of the packages in the config file.

	return nil
}

// doRun will run the generate-manifests command
func doRun(_ *cobra.Command, _ []string) error {
	// TODO `make generate-crds-v1` is required PRIOR to running this.
	// How do we do that?????
	return hub.Generate(hub.GenerateConfig{
		NewVersion:      flags.Tag,
		PrevVersion:     flags.PreviousVersion,
		StackVersion:    flags.StackVersion,
		ConfigPath:      flags.Conf,
		ManifestPaths:   flags.YamlManifest,
		TemplatesPath:   flags.Templates,
		RedhatAPIKey:    flags.ApiKey,
		RedhatProjectID: flags.ProjectID,
	})
}
