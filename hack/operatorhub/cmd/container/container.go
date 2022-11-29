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
	"github.com/spf13/viper"

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
		PreRunE:      defaultPreRunE,
		RunE:         DoPreflight,
	}

	publishCmd := &cobra.Command{
		Use:          "publish",
		Short:        "publish existing eck operator container image within quay.io",
		Long:         "Publish existing eck operator container image within quay.io",
		SilenceUsage: true,
		PreRunE:      defaultPreRunE,
		RunE:         DoPublish,
	}

	pushCmd := &cobra.Command{
		Use:          "push",
		Short:        "push eck operator container image to quay.io",
		Long:         "Push eck operator container image to quay.io",
		SilenceUsage: true,
		PreRunE:      defaultPreRunE,
		RunE:         DoPush,
	}

	cmd.PersistentFlags().StringP(
		"api-key",
		"a",
		"",
		"api key to use when communicating with redhat catalog api (API_KEY)",
	)

	cmd.PersistentFlags().StringP(
		"registry-password",
		"r",
		"",
		"registry password used to communicate with Quay.io (REGISTRY_PASSWORD)",
	)

	cmd.PersistentFlags().StringP(
		"project-id",
		"p",
		"",
		"short project id within the redhat technology portal (PROJECT_ID)",
	)

	cmd.PersistentFlags().BoolP(
		"force",
		"F",
		false,
		"force will force the attempted pushing of remote images, even when the exact version is found remotely. (FORCE)",
	)

	cmd.PersistentFlags().Bool(
		"enable-vault",
		false,
		"Enable vault functionality to try and automatically read 'registry-password', 'api-key' and 'project-id' from given vault key (uses VAULT_* environment variables) (ENABLE_VAULT)",
	)

	cmd.PersistentFlags().String(
		"vault-secret",
		"",
		"When --enable-vault is set, attempts to read 'registry-password', and 'api-key' data from given vault secret location",
	)

	cmd.PersistentFlags().String(
		"vault-addr",
		"",
		"Vault address to use when enable-vault is set",
	)

	cmd.PersistentFlags().String(
		"vault-token",
		"",
		"Vault token to use when enable-vault is set",
	)

	publishCmd.Flags().DurationP(
		"scan-timeout",
		"S",
		1*time.Hour,
		"The duration the publish operation will wait on image being scanned before failing the process completely. (SCAN_TIMEOUT)",
	)

	cmd.AddCommand(pushCmd, preflightCmd, publishCmd)

	return cmd
}

// PreRunE are the pre-run operations for the container command
func PreRunE(cmd *cobra.Command, args []string) error {
	if err := defaultPreRunE(cmd, args); err != nil {
		return err
	}

	if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
		return fmt.Errorf("failed to bind persistent flags: %w", err)
	}

	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		return fmt.Errorf("failed to bind flags: %w", err)
	}

	viper.AutomaticEnv()

	if viper.GetBool("enable-vault") {
		if viper.GetString("vault-secret") == "" {
			return fmt.Errorf("vault-secret is required when enable-vault is set")
		}
		return attemptVault()
	}

	if viper.GetString("api-key") == "" {
		return fmt.Errorf("api-key must be set")
	}

	if viper.GetString("registry-password") == "" {
		return fmt.Errorf("registry-password must be set")
	}

	if viper.GetString("project-id") == "" {
		return fmt.Errorf("project-id must be set")
	}

	return nil
}

func defaultPreRunE(cmd *cobra.Command, args []string) error {
	if cmd.Parent() != nil && cmd.Parent().PreRunE != nil {
		if err := cmd.Parent().PreRunE(cmd.Parent(), args); err != nil {
			return err
		}
	}

	if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
		return fmt.Errorf("failed to bind persistent flags: %w", err)
	}

	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		return fmt.Errorf("failed to bind flags: %w", err)
	}

	viper.AutomaticEnv()
	return nil
}

// DoPublish will publish an existing image within the redhat catalog.
func DoPublish(_ *cobra.Command, _ []string) error {
	return pkg_container.PublishImage(pkg_container.PublishConfig{
		DryRun:              viper.GetBool("dry-run"),
		Force:               viper.GetBool("force"),
		ProjectID:           viper.GetString("project-id"),
		RedhatCatalogAPIKey: viper.GetString("api-key"),
		RegistryPassword:    viper.GetString("registry-password"),
		Tag:                 viper.GetString("tag"),
		ImageScanTimeout:    viper.GetDuration("scan-timeout"),
	})
}

func DoPreflight(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	dir, err := os.MkdirTemp(os.TempDir(), "docker_credentials")
	if err != nil {
		return fmt.Errorf("while creating temporary directory for docker credentials: %w", err)
	}
	defer os.RemoveAll(dir)
	err = pkg_container.LoginToRegistry(dir, viper.GetString("project-id"), viper.GetString("registry-password"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	containerImage := fmt.Sprintf("%s/redhat-isv-containers/%s:%s", "quay.io", viper.GetString("project-id"), viper.GetString("tag"))
	results, err := pkg_preflight.Run(
		ctx,
		pkg_preflight.RunInput{
			Image:                  containerImage,
			DockerConfigPath:       filepath.Join(dir, "config.json"),
			PyxisAPIToken:          viper.GetString("api-key"),
			CertificationProjectID: viper.GetString("project-id"),
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
		DryRun:              viper.GetBool("dry-run"),
		Force:               viper.GetBool("force"),
		ProjectID:           viper.GetString("project-id"),
		RedhatCatalogAPIKey: viper.GetString("api-key"),
		RegistryPassword:    viper.GetString("registry-password"),
		Tag:                 viper.GetString("tag"),
	})
}
