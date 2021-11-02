// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/spf13/cobra"

	"github.com/elastic/cloud-on-k8s/hack/deployer/runner"
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
			case runner.GkeDriverID:
				gCloudProject, err := GetEnvVar("GCLOUD_PROJECT")
				if err != nil {
					return err
				}

				cfgData = fmt.Sprintf(runner.DefaultGkeRunConfigTemplate, user, gCloudProject)
			case runner.AksDriverID:
				resourceGroup, err := GetEnvVar("RESOURCE_GROUP")
				if err != nil {
					return err
				}

				cfgData = fmt.Sprintf(runner.DefaultAksRunConfigTemplate, user, resourceGroup)
			case runner.OcpDriverID:
				gCloudProject, err := GetEnvVar("GCLOUD_PROJECT")
				if err != nil {
					return err
				}

				pullSecret, err := GetEnvVar("OCP_PULL_SECRET")
				if err != nil {
					return err
				}

				cfgData = fmt.Sprintf(runner.DefaultOcpRunConfigTemplate, user, gCloudProject, pullSecret)
			case runner.EKSDriverID:
				// optional variable for local dev use
				token, _ := os.LookupEnv("GITHUB_TOKEN")

				vaultAddr, err := GetEnvVar("VAULT_ADDR")
				if err != nil {
					return err
				}

				cfgData = fmt.Sprintf(runner.DefaultEKSRunConfigTemplate, user, vaultAddr, token)
			case runner.KindDriverID:
				cfgData = fmt.Sprintf(runner.DefaultKindRunConfigTemplate, user)
			case runner.TanzuDriverID:
				cfgData = fmt.Sprintf(runner.DefaultTanzuRunConfigTemplate, user)
			default:
				return fmt.Errorf("unknown provider %s", provider)
			}

			fullPath := path.Join(filePath, fmt.Sprintf("deployer-config-%s.yml", provider))
			return ioutil.WriteFile(fullPath, []byte(cfgData), 0600)
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
