// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"os"

	"github.com/spf13/cobra"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("certificate-initializer")

func main() {
	logf.SetLogger(logf.ZapLogger(true))

	cmd := &cobra.Command{
		Use:   "cert-initializer",
		Short: "Start the certificate initializer",
		Long:  `Start an HTTP server serving a generated CSR`,
		Run: func(cmd *cobra.Command, args []string) {

			cfg := NewConfigFromFlags()
			certInit := NewCertInitializer(cfg)

			err := certInit.Start()
			exitOnErr(err)
		},
	}

	err := BindEnvFromFlags(cmd)
	exitOnErr(err)

	err = cmd.Execute()
	exitOnErr(err)
}

// exitOnErr exits the program if err exists.
func exitOnErr(err error) {
	if err != nil {
		log.Error(err, "Fatal error")
		os.Exit(1)
	}
}
