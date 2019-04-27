// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"os"

	"github.com/elastic/k8s-operators/operators/cmd/cli/license"
	"github.com/spf13/cobra"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	name = "cli"
	log  = logf.Log.WithName(name)
)

func main() {
	rootCmd := &cobra.Command{
		Use: name,
	}
	rootCmd.AddCommand(license.Cmd)
	cobra.OnInitialize(func() {
		logf.SetLogger(logf.ZapLogger(true))
	})
	if err := rootCmd.Execute(); err != nil {
		log.Error(err, "Command failed")
		os.Exit(1)
	}
}
