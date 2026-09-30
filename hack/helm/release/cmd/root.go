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
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/vault"
)

const (
	// viper flags
	chartsDirFlag            = "charts-dir"
	credentialsFileFlag      = "credentials-file"
	dryRunFlag               = "dry-run"
	forceFlag                = "force"
	keepTmpDirFlag           = "keep-tmp-dir"
	envFlag                  = "env"
	enableVaultFlag          = "enable-vault"
	skipChartRepoFlag        = "skip-chart-repo"
	skipOCIRegistryFlag      = "skip-oci-registry"
	ociChartsDigestsFileFlag = "oci-charts-digests-file"

	// GCS Helm Buckets
	devBucket  = "elastic-helm-charts-dev"
	prodBucket = "elastic-helm-charts"

	// Helm Repositories URL
	devRepoURL  = "https://helm-dev.elastic.co/helm"
	prodRepoURL = "https://helm.elastic.co/helm"

	// OCI registries
	devOCIRegistry  = "docker.elastic.co/eck-snapshots"
	prodOCIRegistry = "docker.elastic.co/eck"

	// Environment flag options
	devEnvironment  = "dev"
	prodEnvironment = "prod"

	googleCredsVaultSecretPath = "helm-gcs-credentials"
	googleCredsVaultSecretKey  = "creds.json"
	googleCredentialsEnvVar    = "GOOGLE_APPLICATION_CREDENTIALS"

	ociCredsVaultSecretPath  = "docker-registry-elastic"
	ociCredsVaultUsernameKey = "username"
	ociCredsVaultPasswordKey = "password"
)

var (
	bucket        string
	chartsRepoURL string
	ociRegistry   string
	ociUsername   string
	ociPassword   string
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
			var destinations []string
			skipChartRepo := viper.GetBool(skipChartRepoFlag)
			skipOCIRegistry := viper.GetBool(skipOCIRegistryFlag)
			if skipChartRepo && skipOCIRegistry {
				log.Printf("Nothing to release: both --%s and --%s are set", skipChartRepoFlag, skipOCIRegistryFlag)
				return nil
			}
			if !skipChartRepo {
				destinations = append(destinations, fmt.Sprintf("bucket (%s) / repo (%s)", bucket, chartsRepoURL))
			}
			if !skipOCIRegistry {
				destinations = append(destinations, fmt.Sprintf("OCI registry (%s)", ociRegistry))
			}
			log.Printf("Releasing charts in (%s) to %s\n", viper.GetString(chartsDirFlag), strings.Join(destinations, " and "))
			return helm.Release(
				helm.ReleaseConfig{
					ChartsDir:                viper.GetString(chartsDirFlag),
					Bucket:                   bucket,
					ChartsRepoURL:            chartsRepoURL,
					CredentialsFilePath:      viper.GetString(credentialsFileFlag),
					DryRun:                   viper.GetBool(dryRunFlag),
					Force:                    viper.GetBool(forceFlag),
					KeepTmpDir:               viper.GetBool(keepTmpDirFlag),
					IsProdRelease:            viper.GetString(envFlag) == prodEnvironment,
					SkipChartRepo:            viper.GetBool(skipChartRepoFlag),
					SkipOCIRegistry:          viper.GetBool(skipOCIRegistryFlag),
					OCIRegistry:              ociRegistry,
					OCIUsername:              ociUsername,
					OCIPassword:              ociPassword,
					OCIChartsDigestsFilePath: viper.GetString(ociChartsDigestsFileFlag),
				})
		},
	}

	flags := releaseCommand.Flags()

	flags.BoolP(
		dryRunFlag,
		"d",
		true,
		"Do not upload files to bucket, update Helm index, or push to OCI registry (env: HELM_DRY_RUN)",
	)
	_ = viper.BindPFlag(dryRunFlag, flags.Lookup(dryRunFlag))

	flags.BoolP(
		forceFlag,
		"f",
		false,
		"Upload artifacts even if they already exist (env: HELM_FORCE)",
	)
	_ = viper.BindPFlag(forceFlag, flags.Lookup(forceFlag))

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
		"Read 'credentials-file' and the OCI registry credentials from Vault (requires VAULT_ADDR and VAULT_TOKEN). When disabled, the local Docker/Helm registry login is used (env: HELM_ENABLE_VAULT)",
	)
	_ = viper.BindPFlag(enableVaultFlag, flags.Lookup(enableVaultFlag))

	flags.Bool(
		skipChartRepoFlag,
		false,
		"Skip uploading to the GCS bucket and updating the Helm index. Useful when only OCI publishing is needed (env: HELM_SKIP_CHART_REPO)",
	)
	_ = viper.BindPFlag(skipChartRepoFlag, flags.Lookup(skipChartRepoFlag))

	flags.Bool(
		skipOCIRegistryFlag,
		false,
		"Skip pushing charts to the OCI registry. Useful when only GCS publishing is needed (env: HELM_SKIP_OCI_REGISTRY)",
	)
	_ = viper.BindPFlag(skipOCIRegistryFlag, flags.Lookup(skipOCIRegistryFlag))

	flags.String(
		ociChartsDigestsFileFlag,
		"",
		"Path to a file where pushed OCI chart digest refs are written, one per line (e.g. registry/chart:version@sha256:...). Empty to skip (env: HELM_OCI_CHARTS_DIGESTS_FILE)",
	)
	_ = viper.BindPFlag(ociChartsDigestsFileFlag, flags.Lookup(ociChartsDigestsFileFlag))

	return releaseCommand
}

func validate(_ *cobra.Command, _ []string) error {
	env := viper.GetString(envFlag)
	switch env {
	case devEnvironment:
		bucket = devBucket
		chartsRepoURL = devRepoURL
		ociRegistry = devOCIRegistry
	case prodEnvironment:
		bucket = prodBucket
		chartsRepoURL = prodRepoURL
		ociRegistry = prodOCIRegistry
	default:
		return fmt.Errorf("%s flag can only be on of (%s, %s)", envFlag, devEnvironment, prodEnvironment)
	}

	enableVault := viper.GetBool(enableVaultFlag)
	vaultClientProvider := vault.NewClientProvider()
	if enableVault && !viper.GetBool(skipOCIRegistryFlag) {
		vaultClient, err := vaultClientProvider()
		if err != nil {
			return fmt.Errorf("while creating vault client: %w", err)
		}
		creds, err := vault.GetMany(vaultClient, ociCredsVaultSecretPath, ociCredsVaultUsernameKey, ociCredsVaultPasswordKey)
		if err != nil {
			return fmt.Errorf("while reading OCI registry credentials from vault: %w", err)
		}
		ociUsername, ociPassword = creds[0], creds[1]
	}

	if !viper.GetBool(skipChartRepoFlag) {
		credentialsFilePath := viper.GetString(credentialsFileFlag)
		if credentialsFilePath == "" {
			return fmt.Errorf("%s is a required flag", credentialsFileFlag)
		}
		if enableVault {
			_, err := vault.ReadFile(vaultClientProvider, vault.SecretFile{
				Name:          credentialsFilePath,
				Path:          googleCredsVaultSecretPath,
				FieldResolver: func() string { return googleCredsVaultSecretKey },
			})
			if err != nil {
				return fmt.Errorf("while reading '%s' from vault: %w", credentialsFilePath, err)
			}
		}

		if _, err := os.Stat(credentialsFilePath); err != nil {
			return fmt.Errorf("while accessing google credentials file (%s): %w", credentialsFilePath, err)
		}
		os.Setenv(googleCredentialsEnvVar, credentialsFilePath)
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
