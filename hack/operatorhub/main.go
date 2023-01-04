// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	"log"
	"os"

	root "github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd"
)

func main() {
	if err := root.Cmd.Execute(); err != nil {
		log.Printf("failed to run command: %s", err)
		os.Exit(1)
	}
}
