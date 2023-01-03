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
)

const (
	// viper flags
	bucketFlag          = "bucket"
	chartsDirFlag       = "charts-dir"
	chartsRepoURLFlag   = "charts-repo-url"
	credentialsFileFlag = "credentials-file"
	dryRunFlag          = "dry-run"
	envFlag             = "env"
	excludesFlag        = "excludes"
	enableVaultFlag     = "enable-vault"
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
		Example: fmt.Sprintf("  %s", "release --charts-dir=./deploy --upload-index --dry-run=false"),
		PreRunE: validate,
		RunE: func(_ *cobra.Command, _ []string) error {
			log.Printf("Releasing charts in (%s) to bucket (%s) in repo (%s)\n", viper.GetString("charts-dir"), viper.GetString("bucket"), viper.GetString("charts-repo-url"))
			return helm.Release(
				helm.ReleaseConfig{
					ChartsDir:           viper.GetString("charts-dir"),
					Bucket:              bucket,
					ChartsRepoURL:       chartsRepoURL,
					CredentialsFilePath: viper.GetString("credentials-file"),
					DryRun:              viper.GetBool("dry-run"),
					Excludes:            viper.GetStringSlice("excludes"),
				})
		},
	}

	flags := releaseCommand.Flags()

	flags.BoolP(
		dryRunFlag,
		"d",
		true,
		"Do not update upload new files to bucket, or update helm index (env: HELM_DRY_RUN)",
	)
	_ = viper.BindPFlag(dryRunFlag, flags.Lookup(dryRunFlag))

	flags.String(
		chartsDirFlag,
		"./deploy",
		"Charts directory which contain helm charts (env: HELM_CHARTS_DIR)",
	)
	_ = viper.BindPFlag(chartsDirFlag, flags.Lookup(chartsDirFlag))

	flags.String(
		credentialsFileFlag,
		"",
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
		false,
		"Enable vault functionality to try and automatically read 'credentials-file' from given vault key (uses HELM_VAULT_* environment variables) (HELM_ENABLE_VAULT)",
	)
	_ = viper.BindPFlag(enableVaultFlag, flags.Lookup(enableVaultFlag))

	flags.String(
		vaultSecretFlag,
		"",
		"When --enable-vault is set, attempts to read 'credentials-file' data from given vault secret location (HELM_VAULT_SECRET)",
	)
	_ = viper.BindPFlag(vaultSecretFlag, flags.Lookup(vaultSecretFlag))

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

	if viper.GetBool(enableVaultFlag) {
		if viper.GetString(vaultSecretFlag) == "" {
			return fmt.Errorf("%s is required when %s is set", vaultSecretFlag, enableVaultFlag)
		}
		err := readCredentialsFromVault()
		if err != nil {
			return err
		}
	}

	if viper.GetString(credentialsFileFlag) == "" {
		return fmt.Errorf("%s is a required flag", credentialsFileFlag)
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
