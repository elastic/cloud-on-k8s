// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"github.com/elastic/cloud-on-k8s/cmd/manager"
	"github.com/elastic/cloud-on-k8s/pkg/dev"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("main")

func main() {
	var rootCmd = &cobra.Command{Use: "elastic-operator"}
	logLevelFlag := "enable-debug-logs"
	rootCmd.AddCommand(manager.Cmd)
	// development mode is only available as a command line flag to avoid accidentally enabling it
	rootCmd.PersistentFlags().BoolVar(&dev.Enabled, "development", false, "turns on development mode")
	rootCmd.PersistentFlags().Bool(logLevelFlag, false, "If true, enables debug logs. Defaults to false")

	cobra.OnInitialize(func() {
		debug, _ := rootCmd.Flags().GetBool(logLevelFlag)
		logf.SetLogger(logf.ZapLogger(debug))
	})

	if err := rootCmd.Execute(); err != nil {
		log.Error(err, "Unexpected error while executing command")
	}
}
