// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/elastic/cloud-on-k8s/hack/helm/release/internal/helm"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/vault"
)

const (
	// viper flags
	chartsDirFlag       = "charts-dir"
	credentialsFileFlag = "credentials-file"
	dryRunFlag          = "dry-run"
	keepTmpDirFlag      = "keep-tmp-dir"
	envFlag             = "env"
	enableVaultFlag     = "enable-vault"

	// GCS Helm Buckets
	devBucket  = "elastic-helm-charts-dev"
	prodBucket = "elastic-helm-charts"

	// Helm Repositories URL
	devRepoURL  = "https://helm-dev.elastic.co/helm"
	prodRepoURL = "https://helm.elastic.co/helm"

	// Environment flag options
	devEnvironment  = "dev"
	prodEnvironment = "prod"

	googleCredsVaultSecretPath = "helm-gcs-credentials"
	googleCredsVaultSecretKey  = "creds.json"
	googleCredentialsEnvVar    = "GOOGLE_APPLICATION_CREDENTIALS"
)

var (
	bucket        string
	chartsRepoURL string
)

func init() {
	cobra.OnInitialize(initConfig)
}

func releaseCmd() *cobra.Command {
	releaseCommand := &cobra.Command{
		Use:     "release",
		Short:   "Release ECK Helm Charts",
		Example: fmt.Sprintf("  %s", "release --env=prod --charts-dir=./deploy --dry-run=false"),
		PreRunE: validate,
		RunE: func(_ *cobra.Command, _ []string) error {
			log.Printf("Releasing charts in (%s) to bucket (%s) in repo (%s)\n", viper.GetString(chartsDirFlag), bucket, chartsRepoURL)
			return helm.Release(
				helm.ReleaseConfig{
					ChartsDir:           viper.GetString(chartsDirFlag),
					Bucket:              bucket,
					ChartsRepoURL:       chartsRepoURL,
					CredentialsFilePath: viper.GetString(credentialsFileFlag),
					DryRun:              viper.GetBool(dryRunFlag),
					KeepTmpDir:          viper.GetBool(keepTmpDirFlag),
				})
		},
	}

	flags := releaseCommand.Flags()

	flags.BoolP(
		dryRunFlag,
		"d",
		true,
		"Do not upload files to bucket, or update Helm index (env: HELM_DRY_RUN)",
	)
	_ = viper.BindPFlag(dryRunFlag, flags.Lookup(dryRunFlag))

	flags.BoolP(
		keepTmpDirFlag,
		"k",
		false,
		"Keep temporary directory which contains the Helm charts ready to be published (env: HELM_KEEP_TMP_DIR)",
	)
	_ = viper.BindPFlag(keepTmpDirFlag, flags.Lookup(keepTmpDirFlag))

	flags.String(
		chartsDirFlag,
		"./deploy",
		"Directory which contains Helm charts to release (env: HELM_CHARTS_DIR)",
	)
	_ = viper.BindPFlag(chartsDirFlag, flags.Lookup(chartsDirFlag))

	flags.String(
		credentialsFileFlag,
		"/tmp/credentials.json",
		"Path to GCS credentials JSON file (env: HELM_CREDENTIALS_FILE)",
	)
	_ = viper.BindPFlag(credentialsFileFlag, flags.Lookup(credentialsFileFlag))

	flags.String(
		envFlag,
		devEnvironment,
		"Environment in which to release Helm charts ('dev' or 'prod') (env: HELM_ENV)",
	)
	_ = viper.BindPFlag(envFlag, flags.Lookup(envFlag))

	flags.Bool(
		enableVaultFlag,
		true,
		"Read 'credentials-file' from Vault (requires VAULT_ADDR and VAULT_TOKEN) (env: HELM_ENABLE_VAULT)",
	)
	_ = viper.BindPFlag(enableVaultFlag, flags.Lookup(enableVaultFlag))

	return releaseCommand
}

func validate(_ *cobra.Command, _ []string) error {
	env := viper.GetString(envFlag)
	switch env {
	case devEnvironment:
		bucket = devBucket
		chartsRepoURL = devRepoURL
	case prodEnvironment:
		bucket = prodBucket
		chartsRepoURL = prodRepoURL
	default:
		return fmt.Errorf("%s flag can only be on of (%s, %s)", envFlag, devEnvironment, prodEnvironment)
	}

	credentialsFilePath := viper.GetString(credentialsFileFlag)
	if credentialsFilePath == "" {
		return fmt.Errorf("%s is a required flag", credentialsFilePath)
	}

	if viper.GetBool(enableVaultFlag) {
		c := vault.NewClientProvider()
		_, err := vault.ReadFile(c, vault.SecretFile{
			Name:          credentialsFilePath,
			Path:          googleCredsVaultSecretPath,
			FieldResolver: func() string { return googleCredsVaultSecretKey },
		})
		if err != nil {
			return fmt.Errorf("while reading '%s' from vault: %w", credentialsFilePath, err)
		}
	}

	_, err := os.Open(credentialsFilePath)
	if err != nil {
		return fmt.Errorf("while reading google credentials file (%s): %w", credentialsFilePath, err)
	}
	os.Setenv(googleCredentialsEnvVar, credentialsFilePath)

	return nil
}

// Execute will execute the Helm release flow.
func Execute() {
	err := releaseCmd().Execute()
	if err != nil {
		os.Exit(1)
	}
}

func initConfig() {
	// set up environment variable support
	viper.SetEnvPrefix("helm")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
