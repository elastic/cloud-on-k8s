// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"fmt"
	"os"
	"path"

	"github.com/spf13/cobra"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/runner"
)

func CreateCommand() *cobra.Command {
	var filePath string
	var provider string

	var createCommand = &cobra.Command{
		Use:   "create",
		Short: "Creates run config file(s).",
	}
	var createDefaultConfigCommand = &cobra.Command{
		Use:   "defaultConfig",
		Short: "Creates default dev config using env variables required for chosen provider.",
		RunE: func(cmd *cobra.Command, args []string) error {
			user, err := GetEnvVar("USER")
			if err != nil {
				return err
			}
			var cfgData string
			switch provider {
			case runner.GKEDriverID:
				gCloudProject, err := GetEnvVar("GCLOUD_PROJECT")
				if err != nil {
					return err
				}

				cfgData = fmt.Sprintf(runner.DefaultGKERunConfigTemplate, user, gCloudProject)
			case runner.AKSDriverID:
				resourceGroup, err := GetEnvVar("RESOURCE_GROUP")
				if err != nil {
					return err
				}

				cfgData = fmt.Sprintf(runner.DefaultAKSRunConfigTemplate, user, resourceGroup)
			case runner.OCPDriverID:
				gCloudProject, err := GetEnvVar("GCLOUD_PROJECT")
				if err != nil {
					return err
				}

				cfgData = fmt.Sprintf(runner.DefaultOCPRunConfigTemplate, user, gCloudProject)
			case runner.EKSDriverID:
				// optional variables for local dev use: preferably login to vault external to deployer and export VAULT_ADDR
				token, _ := os.LookupEnv("GITHUB_TOKEN")
				vaultAddr, _ := GetEnvVar("VAULT_ADDR")

				cfgData = fmt.Sprintf(runner.DefaultEKSRunConfigTemplate, user, vaultAddr, token)
			case runner.KindDriverID:
				cfgData = fmt.Sprintf(runner.DefaultKindRunConfigTemplate, user)
			default:
				return fmt.Errorf("unknown provider %s", provider)
			}

			fullPath := path.Join(filePath, fmt.Sprintf("deployer-config-%s.yml", provider))
			return os.WriteFile(fullPath, []byte(cfgData), 0600)
		},
	}

	createDefaultConfigCommand.Flags().StringVar(&filePath, "path", "config/", "Path where files should be created.")
	createDefaultConfigCommand.Flags().StringVar(&provider, "provider", "gke", "Provider to use.")
	createCommand.AddCommand(createDefaultConfigCommand)

	return createCommand
}

func GetEnvVar(name string) (string, error) {
	val, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("%s environment variable not present", name)
	}

	return val, nil
}
