/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
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
		RunE:    func(_ *cobra.Command, _ []string) error { return helm.Release() },
	}

	flags := releaseCommand.Flags()

	flags.BoolP("upload-index", "u", false, "update and upload new helm index")
	_ = viper.BindPFlag("upload-index", flags.Lookup("upload-index"))

	flags.String("charts-dir", "./deploy", "charts directory which contain helm charts (env: HELM_CHARTS_DIR)")
	_ = viper.BindPFlag("charts-dir", flags.Lookup("charts-dir"))

	flags.String("bucket", "elastic-helm-charts-dev", "GCS bucket in which to release helm charts (env: HELM_BUCKET)")
	_ = viper.BindPFlag("bucket", flags.Lookup("bucket"))

	flags.String("charts-repo-url", "https://helm-dev.elastic.co/helm", "URL of Helm Charts Repository (env: HELM_CHARTS_REPO_URL)")
	_ = viper.BindPFlag("charts-repo-url", flags.Lookup("charts-repo-url"))

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
