// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates/cert-initializer"
	"github.com/spf13/cobra"
	"os"
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
			cfg := certinitializer.NewConfig()
			certInit := certinitializer.NewCertInitializer(cfg)
			err := certInit.Start(true)
			exitOnErr(err)
		},
	}

	err := certinitializer.BindEnv(cmd)
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
