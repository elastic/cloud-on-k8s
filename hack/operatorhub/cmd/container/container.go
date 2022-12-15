// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/redhat-openshift-ecosystem/openshift-preflight/certification/formatters"
	"github.com/spf13/cobra"

	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/flags"
	pkg_container "github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/pkg/container"
	pkg_preflight "github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/pkg/preflight"
)

// Command will return the container command
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "container",
		Short:        "push and publish eck operator container to quay.io",
		Long:         "Push and/or Publish eck operator container image to quay.io",
		PreRunE:      PreRunE,
		SilenceUsage: true,
	}

	preflightCmd := &cobra.Command{
		Use:          "preflight",
		Short:        "run preflight tests against container",
		Long:         "Run preflight tests against container",
		SilenceUsage: true,
		RunE:         DoPreflight,
	}

	publishCmd := &cobra.Command{
		Use:          "publish",
		Short:        "publish existing eck operator container image within quay.io",
		Long:         "Publish existing eck operator container image within quay.io",
		SilenceUsage: true,
		RunE:         DoPublish,
	}

	pushCmd := &cobra.Command{
		Use:          "push",
		Short:        "push eck operator container image to quay.io",
		Long:         "Push eck operator container image to quay.io",
		SilenceUsage: true,
		RunE:         DoPush,
	}

	cmd.PersistentFlags().StringVarP(
		&flags.ApiKey,
		flags.ApiKeyFlags,
		"a",
		"",
		"api key to use when communicating with redhat catalog api (OHUB_API_KEY)",
	)

	cmd.PersistentFlags().StringVarP(
		&flags.RegistryPassword,
		flags.RegistryPasswordFlag,
		"r",
		"",
		"registry password used to communicate with Quay.io (OHUB_REGISTRY_PASSWORD)",
	)

	cmd.PersistentFlags().StringVarP(
		&flags.ProjectID,
		flags.ProjectIDFlag,
		"p",
		"",
		"short project id within the redhat technology portal (OHUB_PROJECT_ID)",
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

	cmd.AddCommand(pushCmd, preflightCmd, publishCmd)

	return cmd
}

// PreRunE are the pre-run operations for the container command
func PreRunE(cmd *cobra.Command, args []string) error {
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

// DoPublish will publish an existing image within the redhat catalog.
func DoPublish(_ *cobra.Command, _ []string) error {
	return pkg_container.PublishImage(pkg_container.PublishConfig{
		DryRun:              flags.DryRun,
		Force:               flags.Force,
		ProjectID:           flags.ProjectID,
		RedhatCatalogAPIKey: flags.ApiKey,
		RegistryPassword:    flags.RegistryPassword,
		Tag:                 flags.Tag,
		ImageScanTimeout:    flags.ScanTimeout,
	})
}

func DoPreflight(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	dir, err := os.MkdirTemp(os.TempDir(), "docker_credentials")
	if err != nil {
		return fmt.Errorf("while creating temporary directory for docker credentials: %w", err)
	}
	defer os.RemoveAll(dir)

	err = pkg_container.LoginToRegistry(dir, flags.ProjectID, flags.RegistryPassword)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	containerImage := fmt.Sprintf("%s/redhat-isv-containers/%s:%s", "quay.io", flags.ProjectID, flags.Tag)
	log.Printf("running preflight for container image: %s", containerImage)
	results, err := pkg_preflight.Run(
		ctx,
		pkg_preflight.RunInput{
			Image:                  containerImage,
			DockerConfigPath:       filepath.Join(dir, "config.json"),
			PyxisAPIToken:          flags.ApiKey,
			CertificationProjectID: flags.ProjectID,
		})
	if err != nil {
		return err
	}
	formatter, err := formatters.NewByName(formatters.DefaultFormat)
	if err != nil {
		return fmt.Errorf("while creating new formatater for preflight output: %w", err)
	}
	output, err := formatter.Format(ctx, results)
	if err != nil {
		return fmt.Errorf("while formatting preflight output: %w", err)
	}
	if !results.PassedOverall {
		return fmt.Errorf("preflight certification failed: %s", string(output))
	}
	log.Printf("preflight succeeded: %s", string(output))
	return nil
}

// DoPush will push an image to the redhat registry for scanning.
func DoPush(_ *cobra.Command, _ []string) error {
	return pkg_container.PushImage(pkg_container.PushConfig{
		DryRun:              flags.DryRun,
		Force:               flags.Force,
		ProjectID:           flags.ProjectID,
		RedhatCatalogAPIKey: flags.ApiKey,
		RegistryPassword:    flags.RegistryPassword,
		Tag:                 flags.Tag,
	})
}
