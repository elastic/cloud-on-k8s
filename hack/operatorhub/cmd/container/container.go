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
		Long:         "Push and Publish eck operator container image to quay.io.",
		PreRunE:      preRunE,
		SilenceUsage: true,
	}

	preflightCmd := &cobra.Command{
		Use:   "preflight",
		Short: "run preflight tests against container",
		Long: `Run preflight tests against container.
Note: This does not publish the test results upstream.`,
		RunE:         doPreflight,
		SilenceUsage: true,
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

	cmd.AddCommand(pushCmd, preflightCmd, publishCmd)

	return cmd
}

// preRunE are the pre-run operations for the container command
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

// doPreflight will execute the preflight operations against a container image
// running the verification steps to ensure that the image meets certain criteria
// prior to being able to publish an image.
//
// *Note* this operation does not currently submit preflight results upstream to Red Hat.
// The team working on the 'openshift-preflight' tool, aren't wanting to support submitting
// results when the tool is used as a library.
// https://github.com/redhat-openshift-ecosystem/openshift-preflight/issues/845#issuecomment-1332797435
func doPreflight(cmd *cobra.Command, _ []string) error {
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
