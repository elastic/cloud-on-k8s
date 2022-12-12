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
	// flags
	bucketFlag          = "bucket"
	chartsDirFlag       = "charts-dir"
	chartsRepoURLFlag   = "charts-repo-url"
	credentialsFileFlag = "credentials-file"
	dryRunFlag          = "dry-run"
	excludesFlag        = "excludes"
	enableVaultFlag     = "enable-vault"
	vaultSecretFlag     = "vault-secret"

	// GCS Helm Buckets
	elasticHelmChartsDevBucket  = "elastic-helm-charts-dev"
	elasticHelmChartsProdBucket = "elastic-helm-charts"

	// Helm Repositories
	elasticHelmChartsDevRepoURL  = "https://helm-dev.elastic.co/helm"
	elasticHelmChartsProdRepoURL = "https://helm.elastic.co/helm"
)

func init() {
	cobra.OnInitialize(initConfig)
}

func releaseCmd() *cobra.Command {
	releaseCommand := &cobra.Command{
		Use:     "release",
		Short:   "Release ECK Helm Charts",
		Long:    `This command will Release ECK Helm Charts to a given GCS Bucket.`,
		Example: fmt.Sprintf("  %s", "release --charts-dir=./deploy --upload-index --dry-run=false"),
		PreRunE: validate,
		RunE: func(_ *cobra.Command, _ []string) error {
			log.Printf("Releasing charts in (%s) to bucket (%s) in repo (%s)\n", viper.GetString("charts-dir"), viper.GetString("bucket"), viper.GetString("charts-repo-url"))
			return helm.Release(
				helm.ReleaseConfig{
					ChartsDir:           viper.GetString("charts-dir"),
					Bucket:              viper.GetString("bucket"),
					ChartsRepoURL:       viper.GetString("charts-repo-url"),
					CredentialsFilePath: viper.GetString("credentials-file"),
					UploadIndex:         viper.GetBool("upload-index"),
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
		"charts directory which contain helm charts (env: HELM_CHARTS_DIR)",
	)
	_ = viper.BindPFlag(chartsDirFlag, flags.Lookup(chartsDirFlag))

	flags.String(
		bucketFlag,
		elasticHelmChartsDevBucket,
		"GCS bucket in which to release helm charts (env: HELM_BUCKET)",
	)
	_ = viper.BindPFlag(bucketFlag, flags.Lookup(bucketFlag))

	flags.String(
		chartsRepoURLFlag,
		elasticHelmChartsDevRepoURL,
		"URL of Helm Charts Repository (env: HELM_CHARTS_REPO_URL)",
	)
	_ = viper.BindPFlag(chartsRepoURLFlag, flags.Lookup(chartsRepoURLFlag))

	flags.String(
		credentialsFileFlag,
		"",
		"path to google credentials json file (env: HELM_CREDENTIALS_FILE)",
	)
	_ = viper.BindPFlag(credentialsFileFlag, flags.Lookup(credentialsFileFlag))

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
	if viper.GetBool(enableVaultFlag) {
		if viper.GetString(vaultSecretFlag) == "" {
			return fmt.Errorf("%s is required when %s is set", vaultSecretFlag, enableVaultFlag)
		}
		return attemptVault()
	}
	if viper.GetString(credentialsFileFlag) == "" {
		return fmt.Errorf("%s is a required flag", credentialsFileFlag)
	}
	gcsBucket := viper.GetString(bucketFlag)
	if gcsBucket == "" {
		return fmt.Errorf("%s is a required flag", bucketFlag)
	}
	chartsRepoURL := viper.GetString(chartsRepoURLFlag)
	if chartsRepoURL == "" {
		return fmt.Errorf("%s is a required flag", chartsRepoURLFlag)
	}
	switch chartsRepoURL {
	case elasticHelmChartsDevRepoURL:
		if gcsBucket != elasticHelmChartsDevBucket {
			return fmt.Errorf("%s must be set to %s when %s is set to %s", bucketFlag, elasticHelmChartsDevBucket, chartsRepoURLFlag, elasticHelmChartsDevRepoURL)
		}
	case elasticHelmChartsProdRepoURL:
		if gcsBucket != elasticHelmChartsProdBucket {
			return fmt.Errorf("%s must be set to %s when %s is set to %s", bucketFlag, elasticHelmChartsProdBucket, chartsRepoURLFlag, elasticHelmChartsProdRepoURL)
		}
	default:
		return fmt.Errorf("%s can only be one of (%s, %s)", chartsRepoURLFlag, elasticHelmChartsDevRepoURL, elasticHelmChartsProdRepoURL)
	}
	return nil
}

// Execute release flow.
func Execute() {
	err := releaseCmd().Execute()
	if err != nil {
		os.Exit(1)
	}
}

func initConfig() {
	// set up ENV var support
	viper.SetEnvPrefix("helm")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
