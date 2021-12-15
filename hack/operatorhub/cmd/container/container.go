// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	pkg_container "github.com/elastic/cloud-on-k8s/hack/operatorhub/pkg/container"
)

// Command will return the container command
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "container",
		Short:        "push and publish eck operator container to redhat catalog",
		Long:         "Push and/or Publish eck operator container image to redhat catalog",
		PreRunE:      PreRunE,
		SilenceUsage: true,
	}

	publishCmd := &cobra.Command{
		Use:          "publish",
		Short:        "publish existing eck operator container image within redhat catalog",
		Long:         "Publish existing eck operator container image within redhat catalog",
		SilenceUsage: true,
		PreRunE:      defaultPreRunE,
		RunE:         DoPublish,
	}

	pushCmd := &cobra.Command{
		Use:          "push",
		Short:        "push eck operator container image to redhat catalog",
		Long:         "Push eck operator container image to redhat catalog",
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
		"redhat-connect-registry-key",
		"r",
		"",
		"registry key used to communicate with redhat docker registry (REDHAT_CONNECT_REGISTRY_KEY)",
	)

	cmd.PersistentFlags().StringP(
		"project-id",
		"p",
		"",
		"short project id within the redhat technology portal (PROJECT_ID)",
	)

	cmd.PersistentFlags().StringP(
		"repository-id",
		"R",
		"",
		"repository project id (ospid) within the redhat technology portal (REPOSITORY_ID)",
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
		"Enable vault functionality to try and automatically read 'redhat-connect-registry-key', and 'api-key' from given vault key (uses VAULT_* environment variables) (ENABLE_VAULT)",
	)

	cmd.PersistentFlags().String(
		"vault-secret",
		"",
		"When --enable-vault is set, attempts to read 'redhat-connect-registry-key', and 'api-key' data from given vault secret location",
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

	cmd.AddCommand(pushCmd, publishCmd)

	return cmd
}

// PreRunE are the pre-run operations for the container command
func PreRunE(cmd *cobra.Command, args []string) error {
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

	if viper.GetBool("enable-vault") {
		if viper.GetString("vault-secret") == "" {
			return fmt.Errorf("vault-secret is required when enable-vault is set")
		}
		return attemptVault()
	}

	if viper.GetString("api-key") == "" {
		return fmt.Errorf("api-key must be set")
	}

	if viper.GetString("redhat-connect-registry-key") == "" {
		return fmt.Errorf("redhat-connect-registry-key must be set")
	}

	if viper.GetString("project-id") == "" {
		return fmt.Errorf("project-id must be set")
	}

	if viper.GetString("repository-id") == "" {
		return fmt.Errorf("repository-id must be set")
	}

	if !strings.HasPrefix(viper.GetString("repository-id"), "ospid-") {
		return fmt.Errorf("repository-id doesn't appear to be valid (must begin with 'ospid-')")
	}

	return nil
}

func defaultPreRunE(cmd *cobra.Command, args []string) error {
	if cmd.Parent() != nil && cmd.Parent().PreRunE != nil {
		if err := cmd.Parent().PreRunE(cmd.Parent(), args); err != nil {
			return err
		}
	}
	return nil
}

// DoPublish will publish an existing image within the redhat catalog.
func DoPublish(_ *cobra.Command, _ []string) error {
	return pkg_container.PublishImage(pkg_container.PublishConfig{
		ProjectID:                viper.GetString("project-id"),
		Tag:                      viper.GetString("tag"),
		RedhatCatalogAPIKey:      viper.GetString("api-key"),
		RedhatConnectRegistryKey: viper.GetString("redhat-connect-registry-key"),
		RepositoryID:             viper.GetString("repository-id"),
		Force:                    viper.GetBool("force"),
		ImageScanTimeout:         viper.GetDuration("scan-timeout"),
	})
}

// DoPush will push an image to the redhat registry for scanning.
func DoPush(_ *cobra.Command, _ []string) error {
	return pkg_container.PushImage(pkg_container.PushConfig{
		ProjectID:                viper.GetString("project-id"),
		Tag:                      viper.GetString("tag"),
		RedhatCatalogAPIKey:      viper.GetString("api-key"),
		RedhatConnectRegistryKey: viper.GetString("redhat-connect-registry-key"),
		RepositoryID:             viper.GetString("repository-id"),
		Force:                    viper.GetBool("force"),
	})
}
