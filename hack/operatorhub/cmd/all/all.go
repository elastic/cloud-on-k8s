// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package all

import (
	"github.com/elastic/cloud-on-k8s/hack/operatorhub/cmd/bundle"
	"github.com/elastic/cloud-on-k8s/hack/operatorhub/cmd/container"
	"github.com/elastic/cloud-on-k8s/hack/operatorhub/cmd/operatorhub"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Command will return the 'all' command to run all redhat operatorhub operations
func Command(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "all",
		Short:   "run all redhat operations",
		Long:    "Run all redhat operations: push operator container; create operatorhub manifests; create operator bundle images and create PR to redhat certified operators repository",
		PreRunE: preRunAllE,
		RunE:    allRunE,
	}
	cmd.Flags().AddFlagSet(root.Flags())
	cmd.Flags().AddFlagSet(bundle.Command().Flags())
	cmd.Flags().AddFlagSet(container.Command().Flags())
	cmd.Flags().AddFlagSet(operatorhub.Command().Flags())
	// The all command will automatically provide this flag, so hide it from user.
	cmd.Flags().MarkHidden("dir")
	return cmd
}

func preRunAllE(cmd *cobra.Command, args []string) error {
	if cmd.Parent().PreRunE != nil {
		if err := cmd.Parent().PreRunE(cmd, args); err != nil {
			return err
		}
	}
	if err := container.PreRunE(cmd, args); err != nil {
		return err
	}

	if err := operatorhub.PreRunE(cmd, args); err != nil {
		return err
	}

	if err := bundle.PreRunE(cmd, args); err != nil {
		return err
	}

	return nil
}

func allRunE(cmd *cobra.Command, args []string) error {
	err := container.DoPush(cmd, args)
	if err != nil {
		return err
	}
	err = container.DoPublish(cmd, args)
	if err != nil {
		return err
	}
	err = operatorhub.Run(cmd, args)
	if err != nil {
		return err
	}
	viper.Set("dir", "./certified-operators")
	err = bundle.DoGenerate(cmd, args)
	if err != nil {
		return err
	}
	return bundle.DoCreatePR(cmd, args)
}
