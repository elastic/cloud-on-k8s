// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/elastic/k8s-operators/local-volume/pkg/provisioner"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Short: "Run the local volume provisioner",
	Run: func(cmd *cobra.Command, args []string) {
		if err := provisioner.Start(); err != nil {
			log.Error(err)
		}
	},
}

func main() {
	flag.Parse() // avoid glog complaining about "logging before flag.Parse'
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
