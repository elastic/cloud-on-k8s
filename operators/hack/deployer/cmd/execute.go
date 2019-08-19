// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cmd

import (
	"github.com/elastic/cloud-on-k8s/operators/hack/deployer/runner"
	"github.com/spf13/cobra"
)

func ExecuteCommand() *cobra.Command {
	var plansFile, configFile *string
	var operation string
	var executeCmd = &cobra.Command{
		Use:   "execute",
		Short: "Executes the plan according to plans file, run config file and overrides.",
		RunE: func(cmd *cobra.Command, args []string) error {
			plans, runConfig, err := runner.ParseFiles(*plansFile, *configFile)
			if err != nil {
				return err
			}

			if operation != "" {
				runConfig.Overrides["operation"] = operation
			}

			driver, err := runner.GetDriver(plans.Plans, runConfig)
			if err != nil {
				return err
			}

			return driver.Execute()
		},
	}

	plansFile, configFile = registerFileFlags(executeCmd)
	executeCmd.Flags().StringVar(&operation, "operation", "", "Operation type. This will override config files.")

	return executeCmd
}