// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/elastic/cloud-on-k8s/hack/deployer/runner"
	"github.com/spf13/cobra"
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

			switch provider {
			case runner.GKEDriverID:
				gCloudProject, err := GetEnvVar("GCLOUD_PROJECT")
				if err != nil {
					return err
				}

				data := fmt.Sprintf(runner.DefaultGKERunConfigTemplate, user, gCloudProject)
				fullPath := path.Join(filePath, runner.GKEConfigFileName)
				if err := ioutil.WriteFile(fullPath, []byte(data), 0644); err != nil {
					return err
				}
			case runner.AKSDriverID:
				resourceGroup, err := GetEnvVar("RESOURCE_GROUP")
				if err != nil {
					return err
				}

				acrName, err := GetEnvVar("ACR_NAME")
				if err != nil {
					return err
				}

				data := fmt.Sprintf(runner.DefaultAKSRunConfigTemplate, user, resourceGroup, acrName)
				fullPath := path.Join(filePath, runner.AKSConfigFileName)
				if err := ioutil.WriteFile(fullPath, []byte(data), 0644); err != nil {
					return err
				}
			case runner.OCPDriverID:
				gCloudProject, err := GetEnvVar("GCLOUD_PROJECT")
				if err != nil {
					return err
				}

				pullSecret, err := GetEnvVar("OCP_PULL_SECRET")
				if err != nil {
					return err
				}

				data := fmt.Sprintf(runner.DefaultOCPRunConfigTemplate, user, gCloudProject, pullSecret)
				fullPath := path.Join(filePath, runner.OCPConfigFileName)
				if err := ioutil.WriteFile(fullPath, []byte(data), 0644); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown provider %s", provider)
			}

			return nil
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
