// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"os"

	"github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	rootCmd.AddCommand(run.Command())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
