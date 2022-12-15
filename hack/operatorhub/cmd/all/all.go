// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package all

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/bundle"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/container"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/flags"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/operatorhub"
	"github.com/spf13/cobra"
)

// Command will return the 'all' command to run all redhat operatorhub operations
func Command(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "all",
		Short: "run all operatorhub operations",
		Long:  "Run all operatorhub operations: push+publish operator container; run preflight check; create operatorhub manifests; create operator bundle images and create PR to redhat certified operators repository",
		// PreRunE: preRunAllE,
		RunE: allRunE,
	}
	cmd.Flags().AddFlagSet(root.Flags())
	cmd.Flags().AddFlagSet(bundle.Command().Flags())
	cmd.Flags().AddFlagSet(container.Command().Flags())
	cmd.Flags().AddFlagSet(operatorhub.Command().Flags())
	// The all command will automatically provide this flag, so hide it from user.
	cmd.Flags().MarkHidden("dir")
	return cmd
}

func allRunE(cmd *cobra.Command, args []string) error {
	rootCmd := cmd.Parent()
	if rootCmd == nil {
		return fmt.Errorf("root command while running 'all' command was nil")
	}
	rootCmd.SetArgs(append([]string{"container", "push"}, args...))
	err := rootCmd.Execute()
	// err := container.DoPush(cmd, args)
	if err != nil {
		return err
	}
	rootCmd.SetArgs(append([]string{"container", "preflight"}, args...))
	err = rootCmd.Execute()
	// err = container.DoPreflight(cmd, args)
	if err != nil {
		return err
	}
	rootCmd.SetArgs(append([]string{"container", "publish"}, args...))
	err = rootCmd.Execute()
	// err = container.DoPublish(cmd, args)
	if err != nil {
		return err
	}
	rootCmd.SetArgs(append([]string{"generate-manifests"}, args...))
	err = rootCmd.Execute()
	// err = operatorhub.Run(cmd, args)
	if err != nil {
		return err
	}
	flags.Dir = "./certified-operators"
	rootCmd.SetArgs(append([]string{"bundle", "generate"}, args...))
	err = rootCmd.Execute()
	// err = bundle.DoGenerate(cmd, args)
	if err != nil {
		return err
	}
	rootCmd.SetArgs(append([]string{"bundle", "create-pr"}, args...))
	return rootCmd.Execute()
	// return bundle.DoCreatePR(cmd, args)
}
