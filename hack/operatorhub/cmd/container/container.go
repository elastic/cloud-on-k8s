// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/flags"
	pkg_container "github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/pkg/container"
)

// Command will return the container command
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "container",
		Short:             "push and publish eck operator container to quay.io",
		Long:              "Push and Publish eck operator container image to quay.io.",
		PersistentPreRunE: preRunE,
		SilenceUsage:      true,
	}

	publishCmd := &cobra.Command{
		Use:          "publish",
		Short:        "publish existing eck operator container image within quay.io",
		Long:         "Publish existing eck operator container image within quay.io using the Redhat certification API.",
		RunE:         doPublish,
		SilenceUsage: true,
	}

	pushCmd := &cobra.Command{
		Use:          "push",
		Short:        "push eck operator container image to quay.io",
		RunE:         doPush,
		SilenceUsage: true,
	}

	cmd.PersistentFlags().StringVarP(
		&flags.ApiKey,
		flags.ApiKeyFlags,
		"a",
		"",
		"API key to use when communicating with redhat certification API (OHUB_API_KEY)",
	)

	cmd.PersistentFlags().StringVarP(
		&flags.RegistryPassword,
		flags.RegistryPasswordFlag,
		"r",
		"",
		"registry password used to communicate with Quay.io (OHUB_REGISTRY_PASSWORD)",
	)

	cmd.PersistentFlags().BoolVarP(
		&flags.Force,
		flags.ForceFlag,
		"F",
		false,
		"force will force the attempted pushing of remote images, even when the exact version is found remotely. (OHUB_FORCE)",
	)

	publishCmd.Flags().DurationVarP(
		&flags.ScanTimeout,
		flags.ScanTimeoutFlag,
		"S",
		1*time.Hour,
		"The duration the publish operation will wait on image being scanned before failing the process completely. (OHUB_SCAN_TIMEOUT)",
	)

	cmd.AddCommand(pushCmd, publishCmd)

	return cmd
}

// preRunE are the pre-run operations for the container command and all sub-commands.
func preRunE(cmd *cobra.Command, args []string) error {
	if flags.ApiKey == "" {
		return fmt.Errorf("%s must be set", flags.ApiKeyFlags)
	}

	if flags.RegistryPassword == "" {
		return fmt.Errorf("%s must be set", flags.RegistryPasswordFlag)
	}

	if flags.ProjectID == "" {
		return fmt.Errorf("%s must be set", flags.ProjectIDFlag)
	}

	return nil
}

// doPublish will publish an existing image within the redhat catalog.
func doPublish(_ *cobra.Command, _ []string) error {
	return pkg_container.PublishImage(pkg_container.PublishConfig{
		DryRun:              flags.DryRun,
		ImageScanTimeout:    flags.ScanTimeout,
		ProjectID:           flags.ProjectID,
		RedhatCatalogAPIKey: flags.ApiKey,
		RegistryPassword:    flags.RegistryPassword,
		Tag:                 flags.Tag,
	})
}

// doPush will push an image to the redhat registry for scanning.
func doPush(_ *cobra.Command, _ []string) error {
	return pkg_container.PushImage(pkg_container.PushConfig{
		DryRun:              flags.DryRun,
		Force:               flags.Force,
		ProjectID:           flags.ProjectID,
		RedhatCatalogAPIKey: flags.ApiKey,
		RegistryPassword:    flags.RegistryPassword,
		Tag:                 flags.Tag,
	})
}
