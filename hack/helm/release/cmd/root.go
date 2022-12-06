/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/elastic/cloud-on-k8s/hack/helm/release/pkg/helm"
)

func init() {
	cobra.OnInitialize(initConfig)
}

var (
	configFileUsed bool
)

func releaseCmd() *cobra.Command {
	releaseCommand := &cobra.Command{
		Use:   "release",
		Short: "Release Helm Charts",
		Long:  `This command will Release ECK Helm Charts to a given GCS Bucket.`,
		// this fmt.Sprintf is in place to keep spacing consistent with cobras two spaces that's used in: Usage, Flags, etc
		Example: fmt.Sprintf("  %s", "release --charts-dir=./deploy --bucket=mybucket --upload-index --chart-repo-url=https://helm-dev.elastic.co/helm"),
		PreRunE: func(_ *cobra.Command, _ []string) error { return nil },
		RunE: func(_ *cobra.Command, _ []string) error {
			log.Printf("Releasing charts in (%s) to bucket (%s) in repo (%s)\n", viper.GetString("charts-dir"), viper.GetString("bucket"), viper.GetString("charts-repo-url"))
			return helm.Release(
				helm.ReleaseConfig{
					ChartsDir:           viper.GetString("charts-dir"),
					Bucket:              viper.GetString("bucket"),
					ChartsRepoURL:       viper.GetString("charts-repo-url"),
					CredentialsFilePath: viper.GetString("credentials-file"),
					GCSURL:              viper.GetString("google-gcs-url"),
					UploadIndex:         viper.GetBool("upload-index"),
				})
		},
	}

	flags := releaseCommand.Flags()

	flags.BoolP("upload-index", "u", false, "update and upload new helm index (env: HELM_UPLOAD_INDEX)")
	_ = viper.BindPFlag("upload-index", flags.Lookup("upload-index"))

	// flags.BoolP("update-dependencies", "U", false, "update chart dependencies prior to packaging (env: HELM_UPDATE_DEPENDENCIES)")
	// _ = viper.BindPFlag("update-dependencies", flags.Lookup("update-dependencies"))

	flags.String("charts-dir", "./deploy", "charts directory which contain helm charts (env: HELM_CHARTS_DIR)")
	_ = viper.BindPFlag("charts-dir", flags.Lookup("charts-dir"))

	flags.String("bucket", "elastic-helm-charts-dev", "GCS bucket in which to release helm charts (env: HELM_BUCKET)")
	_ = viper.BindPFlag("bucket", flags.Lookup("bucket"))

	flags.String("charts-repo-url", "https://helm-dev.elastic.co/helm", "URL of Helm Charts Repository (env: HELM_CHARTS_REPO_URL)")
	_ = viper.BindPFlag("charts-repo-url", flags.Lookup("charts-repo-url"))

	flags.String("credentials-file", "", "path to google credentials json file (env: HELM_CREDENTIALS_FILE)")
	_ = viper.BindPFlag("credentials-file", flags.Lookup("credentials-file"))

	flags.String("google-gcs-url", "https://storage.googleapis.com", "Google GCS URL, if wanting to use storage emulation.  Also disable TLS validation. (env: HELM_GOOGLE_GCS_URL)")
	_ = viper.BindPFlag("google-gcs-url", flags.Lookup("google-gcs-url"))

	return releaseCommand
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := releaseCmd().Execute()
	if err != nil {
		os.Exit(1)
	}
}

func initConfig() {
	// set up ENV var support
	viper.SetEnvPrefix("helm")
	viper.AutomaticEnv()

	// set up optional config file support
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	configFileUsed = true
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			configFileUsed = false
		}
	}

	// Set up cluster defaults
	viper.SetDefault("upload-index", false)
	viper.SetDefault("chart-repo-url", "https://helm-dev.elastic.co/helm")
}
