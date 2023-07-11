// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"errors"
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/runner"
)

func CleanupCommand() *cobra.Command {
	var (
		errUnimplemented = errors.New("unimplemented")
		provider         string
		plansFile        string
	)
	var cleanupCmd = &cobra.Command{
		Use:   "cleanup",
		Short: "Runs the cleanup operation to cleanup clusters older than 3 days in the given provider.",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch provider {
			case runner.GKEDriverID:
				return cleanup(plansFile, []string{"gke-ci"}, &runner.GKEDriverFactory{})
			case runner.AKSDriverID:
				return cleanup(plansFile, []string{"aks-ci"}, &runner.AKSDriverFactory{})
			case runner.OCPDriverID:
				return errUnimplemented
			case runner.EKSDriverID:
				return cleanup(plansFile, []string{"eks-ci", "eks-arm-ci"}, &runner.EKSDriverFactory{})
			case runner.KindDriverID:
				return errUnimplemented
			default:
				return fmt.Errorf("unknown provider %s", provider)
			}
		},
	}

	cleanupCmd.Flags().StringVar(&plansFile, "plans-file", "config/plans.yml", "File containing execution plans.")
	cleanupCmd.Flags().StringVar(&provider, "provider", "gke", "Provider to use.")

	return cleanupCmd
}

// cleanup will attempt to cleanup any clusters older than 3 days
func cleanup(plansFile string, planNames []string, driverFactory runner.DriverFactory) error {
	plans, err := runner.ParsePlans(plansFile)
	if err != nil {
		return err
	}
	for _, planName := range planNames {
		var ciPlan *runner.Plan
		for _, plan := range plans.Plans {
			if plan.Id == planName {
				p := plan
				ciPlan = &p
			}
		}
		if ciPlan == nil {
			return fmt.Errorf("couldn't default ci plan %s", planName)
		}
		client, err := driverFactory.Create(*ciPlan)
		if err != nil {
			return err
		}
		clusters, err := client.Cleanup()
		if err != nil {
			return err
		}
		for _, cluster := range clusters {
			log.Printf("deleted cluster: %s", cluster)
		}
	}
	return nil
}
