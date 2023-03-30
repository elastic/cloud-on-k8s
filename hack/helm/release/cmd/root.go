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
	chartsRepoURLFlag   = "charts-repo-url"
	credentialsFileFlag = "credentials-file"
	dryRunFlag          = "dry-run"
	envFlag             = "env"
	excludesFlag        = "excludes"
	enableVaultFlag     = "enable-vault"
	keepTempDirFlag     = "keep-temp-dir"
	vaultSecretFlag     = "vault-secret"

	// GCS Helm Buckets
	elasticHelmChartsDevBucket  = "elastic-helm-charts-dev"
	elasticHelmChartsProdBucket = "elastic-helm-charts"

	// Helm Repositories
	elasticHelmChartsDevRepoURL  = "https://helm-dev.elastic.co/helm"
	elasticHelmChartsProdRepoURL = "https://helm.elastic.co/helm"

	// Environment flag options
	devEnvironment  = "dev"
	prodEnvironment = "prod"

	vaultSecretField = "creds.json"
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
		Long:    `This command will Release ECK Helm Charts to a given Environment.`,
		Example: fmt.Sprintf("  %s", "release --charts-dir=./deploy --dry-run=false"),
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
					Excludes:            viper.GetStringSlice(excludesFlag),
					KeepTempDir:         viper.GetBool(keepTempDirFlag),
				})
		},
	}

	flags := releaseCommand.Flags()

	flags.BoolP(
		dryRunFlag,
		"d",
		true,
		"Do not upload new files to bucket, or update helm index (env: HELM_DRY_RUN)",
	)
	_ = viper.BindPFlag(dryRunFlag, flags.Lookup(dryRunFlag))

	flags.String(
		chartsDirFlag,
		"./deploy",
		"Charts directory which contains helm charts (env: HELM_CHARTS_DIR)",
	)
	_ = viper.BindPFlag(chartsDirFlag, flags.Lookup(chartsDirFlag))

	flags.String(
		credentialsFileFlag,
		"/tmp/credentials.json",
		"Path to google credentials json file (env: HELM_CREDENTIALS_FILE)",
	)
	_ = viper.BindPFlag(credentialsFileFlag, flags.Lookup(credentialsFileFlag))

	flags.String(
		envFlag,
		devEnvironment,
		"Environment in which to release helm charts (env: HELM_ENV)",
	)
	_ = viper.BindPFlag(envFlag, flags.Lookup(envFlag))

	flags.StringSlice(
		excludesFlag,
		[]string{},
		"Names of helm charts to exclude from release. (env: HELM_EXCLUDES)",
	)
	_ = viper.BindPFlag(excludesFlag, flags.Lookup(excludesFlag))

	flags.Bool(
		enableVaultFlag,
		true,
		"Enable vault functionality to try and automatically read 'credentials-file' from given vault key (requires VAULT_ADDR and VAULT-TOKEN and uses HELM_VAULT_* environment variables) (env: HELM_ENABLE_VAULT)",
	)
	_ = viper.BindPFlag(enableVaultFlag, flags.Lookup(enableVaultFlag))

	flags.String(
		vaultSecretFlag,
		"helm-gcs-credentials",
		"When --enable-vault is set, attempts to read 'credentials-file' data from given vault secret location (HELM_VAULT_SECRET)",
	)
	_ = viper.BindPFlag(vaultSecretFlag, flags.Lookup(vaultSecretFlag))

	flags.Bool(
		keepTempDirFlag,
		false,
		"Keep temporary directory after command exits. (env: HELM_KEEP_TEMP_DIR)",
	)
	_ = viper.BindPFlag(keepTempDirFlag, flags.Lookup(keepTempDirFlag))

	return releaseCommand
}

func validate(_ *cobra.Command, _ []string) error {
	env := viper.GetString(envFlag)
	switch env {
	case devEnvironment:
		bucket = elasticHelmChartsDevBucket
		chartsRepoURL = elasticHelmChartsDevRepoURL
	case prodEnvironment:
		bucket = elasticHelmChartsProdBucket
		chartsRepoURL = elasticHelmChartsProdRepoURL
	default:
		return fmt.Errorf("%s flag can only be on of (%s, %s)", envFlag, devEnvironment, prodEnvironment)
	}

	credentialsFile := viper.GetString(credentialsFileFlag)
	if credentialsFile == "" {
		return fmt.Errorf("%s is a required flag", credentialsFileFlag)
	}

	if viper.GetBool(enableVaultFlag) {
		secretPath := viper.GetString(vaultSecretFlag)
		if secretPath == "" {
			return fmt.Errorf("%s is required when %s is set", vaultSecretFlag, enableVaultFlag)
		}

		c, err := vault.NewClient()
		if err != nil {
			return fmt.Errorf("while creating vault client: %w", err)
		}
		_, err = vault.ReadFile(c, vault.SecretFile{
			Name:          credentialsFile,
			Path:          secretPath,
			FieldResolver: func() string { return "creds.json" },
		})
		if err != nil {
			return fmt.Errorf("while reading '%s' from vault: %w", secretPath, err)
		}
	}

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
