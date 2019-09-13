// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cmd

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
)

const defaultRunConfigTemplate = `id: gke-dev
overrides:
  clusterName: %s-dev-cluster
  gke:
    gCloudProject: %s
`

func CreateCommand() *cobra.Command{
	var path string

	var createCommand = &cobra.Command{
		Use:   "create",
		Short: "Creates run config file.",
	}

	var createDefaultConfigCommand = &cobra.Command{
		Use:   "defaultConfig",
		Short: "Creates default dev config using USER and GCLOUD_PROJECT env variables.",
		RunE: func(cmd *cobra.Command, args []string) error {
			user, ok := os.LookupEnv("USER")
			if !ok {
				return fmt.Errorf("USER environment variable not present")
			}

			gCloudProject, ok := os.LookupEnv("GCLOUD_PROJECT")
			if !ok {
				return fmt.Errorf("GCLOUD_PROJECT environment variable not present")
			}

			data := fmt.Sprintf(defaultRunConfigTemplate, user, gCloudProject)
			if err := ioutil.WriteFile(path, []byte(data), 0644); err != nil {
				return err
			}

			return nil
		},
	}

	createDefaultConfigCommand.Flags().StringVar(&path, "path", "config/deployer-config.yml", "Path where file should be created.")
	createCommand.AddCommand(createDefaultConfigCommand)

	return createCommand
}
