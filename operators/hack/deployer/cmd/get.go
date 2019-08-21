// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cmd

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/hack/deployer/runner"
	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	var plansFile, configFile *string
	var getCommand = &cobra.Command{
		Use:   "get",
		Short: "Gets cluster configuration, credentials.",
	}
	plansFile, configFile = registerFileFlags(getCommand)

	var getClusterNameCommand = &cobra.Command{
		Use:   "clusterName",
		Short: "Gets cluster name as per config.",
		RunE: func(cmd *cobra.Command, args []string) error {
			plans, runConfig, err := runner.ParseFiles(*plansFile, *configFile)
			if err != nil {
				return err
			}

			plan, err := runner.GetPlan(plans.Plans, runConfig)
			if err != nil {
				return err
			}

			fmt.Println(plan.ClusterName)
			return nil
		},
	}

	var getCredentialsCommand = &cobra.Command{
		Use:   "credentials",
		Short: "Fetches credentials for the cluster as per config and sets kubectl context to this cluster.",
		RunE: func(cmd *cobra.Command, args []string) error {
			plans, runConfig, err := runner.ParseFiles(*plansFile, *configFile)
			if err != nil {
				return err
			}

			driver, err := runner.GetDriver(plans.Plans, runConfig)
			if err != nil {
				return err
			}

			return driver.GetCredentials()
		},
	}

	var getConfigCommand = &cobra.Command{
		Use:   "config",
		Short: "Gets entire configuration as per config. Be careful, secrets are included and in plain text.",
		RunE: func(cmd *cobra.Command, args []string) error {
			plans, runConfig, err := runner.ParseFiles(*plansFile, *configFile)
			if err != nil {
				return err
			}

			plan, err := runner.GetPlan(plans.Plans, runConfig)
			if err != nil {
				return err
			}

			planYaml, err := yaml.Marshal(plan)
			if err != nil {
				return err
			}

			fmt.Println(string(planYaml))
			return nil
		},
	}

	getCommand.AddCommand(getClusterNameCommand)
	getCommand.AddCommand(getCredentialsCommand)
	getCommand.AddCommand(getConfigCommand)

	return getCommand
}
