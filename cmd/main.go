// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	// By default this sets `GOMEMLIMIT` to 90% of cgroup's memory limit.
	_ "github.com/KimMachineGun/automemlimit"
	"github.com/spf13/cobra"

	keystoreuploader "github.com/elastic/cloud-on-k8s/v3/cmd/keystore-uploader"
	"github.com/elastic/cloud-on-k8s/v3/cmd/manager"
	"github.com/elastic/cloud-on-k8s/v3/pkg/about"
	"github.com/elastic/cloud-on-k8s/v3/pkg/dev"
)

func main() {
	buildInfo := about.GetBuildInfo()

	rootCmd := &cobra.Command{
		Use:          "elastic-operator",
		Short:        "Elastic Cloud on Kubernetes (ECK) operator",
		Version:      buildInfo.VersionString(),
		SilenceUsage: true,
	}
	rootCmd.AddCommand(manager.Command())
	rootCmd.AddCommand(keystoreuploader.Command())

	// development mode is only available as a command line flag to avoid accidentally enabling it
	rootCmd.PersistentFlags().BoolVar(&dev.Enabled, "development", false, "turns on development mode")
	_ = rootCmd.PersistentFlags().MarkHidden("development")

	_ = rootCmd.Execute()
}
