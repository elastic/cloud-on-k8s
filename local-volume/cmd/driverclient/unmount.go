// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(unmountCmd)
}

var unmountCmd = &cobra.Command{
	Use:  "unmount",
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(
			client.Unmount(client.NewCaller(), args),
		)
	},
}
