// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/cmd/chaos"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/cmd/run"
)

func main() {
	rootCmd := &cobra.Command{
		Use:          "e2e",
		Short:        "E2E testing utilities",
		SilenceUsage: true,
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			log.InitLogger()
		},
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("E2E")
	rootCmd.AddCommand(run.Command(), chaos.Command())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
