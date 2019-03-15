// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"github.com/elastic/k8s-operators/operators/cmd/keystore-updater"
	"github.com/spf13/cobra"
	"os"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	name   = "keystore-updater"
	logger = logf.Log.WithName(name)
)

func main() {
	logf.SetLogger(logf.ZapLogger(true))

	logger.Info("Start keystore-updater")

	cmd := &cobra.Command{
		Use: name,
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err, msg := keystore.NewConfigFromFlags(cmd)
			if err != nil {
				logger.Error(err, "Error reading config from flags", "msg", msg)
				os.Exit(1)
			}
			keystore.NewKeystoreUpdater(logger, cfg).Run()
		},
	}

	if err := cmd.Execute(); err != nil {
		logger.Error(err, "Unexpected error while running command")
	}
}
