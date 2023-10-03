// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/runner"
)

func CleanupCommand() *cobra.Command {
	var (
		configFile, clientBuildDefDir, plansFile *string
		olderThan                                time.Duration
		clusterPrefix                            string
	)

	var cleanupCmd = &cobra.Command{
		Use:   "cleanup",
		Short: "Runs the cleanup operation to cleanup clusters older than the given duration in the given provider.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cleanup(*configFile, *plansFile, clusterPrefix, *clientBuildDefDir, olderThan)
		},
	}

	plansFile, configFile, clientBuildDefDir = registerFileFlags(cleanupCmd)

	cleanupCmd.Flags().DurationVar(&olderThan, "older-than", 24*time.Hour, `The minimum age of the clusters to be deleted (valid time units are "s", "m", "h")`)
	cleanupCmd.Flags().StringVar(&clusterPrefix, "cluster-prefix", "eck-e2e", "The E2E Cluster prefix to use for querying for clusters to cleanup.")

	return cleanupCmd
}

// cleanup will attempt to cleanup any clusters older than given 'olderThan' duration.
func cleanup(configFile, plansFile, clusterPrefix, clientBuildDefDir string, olderThan time.Duration) error {
	plans, runConfig, err := runner.ParseFiles(plansFile, configFile)
	if err != nil {
		return err
	}
	driver, err := runner.GetDriver(plans.Plans, runConfig, clientBuildDefDir)
	if err != nil {
		return err
	}
	err = driver.Cleanup(clusterPrefix, olderThan)
	if err != nil {
		return err
	}
	return nil
}
