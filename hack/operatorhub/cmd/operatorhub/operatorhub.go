// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package operatorhub

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	hub "github.com/elastic/cloud-on-k8s/hack/operatorhub/pkg/operatorhub"
)

// Command will return the operatorhub command
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "operatorhub",
		Short:         "Generate Operator Lifecycle Manager format files",
		Example:       "redhat operatorhub -c ./config.yaml -n 1.9.1 -p 1.8.0 -s 7.16.0",
		SilenceErrors: true,
		SilenceUsage:  true,
		PreRunE:       PreRunE,
		RunE:          Run,
	}

	cmd.Flags().StringP(
		"prev-version",
		"V",
		"",
		"Previous version of the operator to populate 'replaces' in operator cluster service version yaml (PREV_VERSION)",
	)

	cmd.Flags().StringP(
		"stack-version",
		"s",
		"",
		"Stack version of Elastic stack used to populate the operator cluster service version yaml (STACK_VERSION)",
	)

	cmd.Flags().StringP(
		"conf",
		"c",
		"./config.yaml",
		"Path to config file to read CRDs, and packages (CONF)",
	)

	cmd.Flags().StringSliceP(
		"yaml-manifest",
		"y",
		nil,
		"Path to installation manifests (YAML_MANIFEST)",
	)

	cmd.Flags().StringP(
		"templates",
		"T",
		"./templates",
		"Path to the templates directory (TEMPLATES)",
	)

	return cmd
}

// PreRunE are the pre-run operations for the operatorhub command
func PreRunE(cmd *cobra.Command, args []string) error {
	if cmd.Parent() != nil && cmd.Parent().PreRunE != nil {
		if err := cmd.Parent().PreRunE(cmd.Parent(), args); err != nil {
			return err
		}
	}

	if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
		return fmt.Errorf("failed to bind persistent flags: %w", err)
	}

	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		return fmt.Errorf("failed to bind flags: %w", err)
	}

	if viper.GetString("conf") == "" {
		return fmt.Errorf("conf is required")
	}

	if viper.GetString("tag") == "" {
		return fmt.Errorf("tag is required")
	}

	if viper.GetString("prev-version") == "" {
		return fmt.Errorf("prev-version is required")
	}

	if viper.GetString("stack-version") == "" {
		return fmt.Errorf("stack-version is required")
	}

	viper.AutomaticEnv()

	return nil
}

// Run will run the operatorhub command
func Run(_ *cobra.Command, _ []string) error {
	return hub.Generate(hub.GenerateConfig{
		NewVersion:    viper.GetString("tag"),
		PrevVersion:   viper.GetString("prev-version"),
		StackVersion:  viper.GetString("stack-version"),
		ConfigPath:    viper.GetString("conf"),
		ManifestPaths: viper.GetStringSlice("yaml-manifest"),
		TemplatesPath: viper.GetString("templates"),
	})
}
