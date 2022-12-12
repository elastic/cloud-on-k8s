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

	flags.BoolP("upload-index", "u", false, "update and upload new helm index (env: HELM_UPLOAD_INDEX)")
	_ = viper.BindPFlag("upload-index", flags.Lookup("upload-index"))

	flags.BoolP("dry-run", "d", true, "Do not update upload new files to bucket, or update helm index (env: HELM_DRY_RUN)")
	_ = viper.BindPFlag("dry-run", flags.Lookup("dry-run"))

	flags.String("charts-dir", "./deploy", "charts directory which contain helm charts (env: HELM_CHARTS_DIR)")
	_ = viper.BindPFlag("charts-dir", flags.Lookup("charts-dir"))

	flags.String("bucket", "elastic-helm-charts-dev", "GCS bucket in which to release helm charts (env: HELM_BUCKET)")
	_ = viper.BindPFlag("bucket", flags.Lookup("bucket"))

	flags.String("charts-repo-url", "https://helm-dev.elastic.co/helm", "URL of Helm Charts Repository (env: HELM_CHARTS_REPO_URL)")
	_ = viper.BindPFlag("charts-repo-url", flags.Lookup("charts-repo-url"))

	flags.String("credentials-file", "", "path to google credentials json file (env: HELM_CREDENTIALS_FILE)")
	_ = viper.BindPFlag("credentials-file", flags.Lookup("credentials-file"))

	flags.StringSlice("excludes", []string{}, "Names of helm charts to exclude from release. (env: HELM_EXCLUDES)")
	_ = viper.BindPFlag("excludes", flags.Lookup("excludes"))

	flags.Bool(
		"enable-vault",
		false,
		"Enable vault functionality to try and automatically read 'credentials-file' from given vault key (uses HELM_VAULT_* environment variables) (HELM_ENABLE_VAULT)",
	)
	_ = viper.BindPFlag("enable-vault", flags.Lookup("enable-vault"))

	flags.String(
		"vault-secret",
		"",
		"When --enable-vault is set, attempts to read 'credentials-file' data from given vault secret location (HELM_VAULT_SECRET)",
	)

	return releaseCommand
}

func validate(_ *cobra.Command, _ []string) error {
	if viper.GetBool("enable-vault") {
		if viper.GetString("vault-secret") == "" {
			return fmt.Errorf("vault-secret is required when enable-vault is set")
		}
		return attemptVault()
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
