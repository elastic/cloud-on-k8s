// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package run

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	logutil "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

type runFlags struct {
	managedNamespaces      []string
	e2eImage               string
	elasticStackVersion    string
	elasticStackImagesPath string
	kubeConfig             string
	operatorImage          string
	testLicensePKeyPath    string
	testContextOutPath     string
	testLicense            string
	scratchDirRoot         string
	testRegex              string
	testRunName            string
	monitoringSecrets      string
	pipeline               string
	buildNumber            string
	provider               string
	clusterName            string
	operatorReplicas       int
	commandTimeout         time.Duration
	logVerbosity           int
	testTimeout            time.Duration
	autoPortForwarding     bool
	skipCleanup            bool
	local                  bool
	logToFile              bool
	ignoreWebhookFailures  bool
	deployChaosJob         bool
	testEnvTags            []string
}

var log logr.Logger

func Command() *cobra.Command {
	flags := runFlags{}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "setup an e2e test environment and run tests",
		PreRunE: func(_ *cobra.Command, _ []string) error {
			log = logf.Log.WithName(flags.testRunName)
			if err := checkWantedDirectories(); err != nil {
				log.Error(err, "Please make sure this command is executed from the root of the repo")
				return err
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags.logVerbosity, _ = cmd.PersistentFlags().GetInt("log-verbosity")
			err := doRun(flags)
			if err != nil {
				log.Error(err, "Failed to run e2e tests")
			}
			return err
		},
	}

	cmd.Flags().BoolVar(&flags.autoPortForwarding, "auto-port-forwarding", false, "Enable port forwarding to pods")
	cmd.Flags().DurationVar(&flags.commandTimeout, "command-timeout", 90*time.Second, "Timeout for commands executed")
	cmd.Flags().StringVar(&flags.e2eImage, "e2e-image", "", "E2E test image")
	cmd.Flags().StringVar(&flags.elasticStackVersion, "elastic-stack-version", test.LatestReleasedVersion7x, "Elastic Stack version")
	cmd.Flags().StringVar(&flags.elasticStackImagesPath, "elastic-stack-images", "", "Path to config file declaring images for individual Elastic Stack applications")
	cmd.Flags().StringVar(&flags.kubeConfig, "kubeconfig", "", "Path to kubeconfig")
	cmd.Flags().BoolVar(&flags.local, "local", false, "Create the environment for running tests locally")
	cmd.Flags().StringSliceVar(&flags.managedNamespaces, "managed-namespaces", []string{"mercury", "venus"}, "List of managed namespaces")
	cmd.Flags().StringVar(&flags.operatorImage, "operator-image", "", "Operator image")
	cmd.Flags().IntVar(&flags.operatorReplicas, "operator-replicas", 1, "Operator replicas")
	cmd.Flags().BoolVar(&flags.skipCleanup, "skip-cleanup", false, "Do not run cleanup actions after test run")
	cmd.Flags().StringVar(&flags.testContextOutPath, "test-context-out", "", "Write the test context to the given path")
	cmd.Flags().StringVar(&flags.testLicense, "test-license", "", "Test license to apply")
	cmd.Flags().StringVar(&flags.testLicensePKeyPath, "test-license-pkey-path", "", "Path to private key to generate test licenses")
	cmd.Flags().StringVar(&flags.monitoringSecrets, "monitoring-secrets", "", "Monitoring secrets to use")
	cmd.Flags().StringVar(&flags.scratchDirRoot, "scratch-dir", "/tmp/eck-e2e", "Path under which temporary files should be created")
	cmd.Flags().StringVar(&flags.testRegex, "test-regex", "", "Regex to pass to the test runner")
	cmd.Flags().StringVar(&flags.testRunName, "test-run-name", randomTestRunName(), "Name of this test run")
	cmd.Flags().DurationVar(&flags.testTimeout, "test-timeout", 30*time.Minute, "Timeout before failing a test")
	cmd.Flags().StringVar(&flags.pipeline, "pipeline", "", "E2E test pipeline name")
	cmd.Flags().StringVar(&flags.buildNumber, "build-number", "", "E2E test build number")
	cmd.Flags().StringVar(&flags.provider, "provider", "", "E2E test infrastructure provider")
	cmd.Flags().StringVar(&flags.clusterName, "clusterName", "", "E2E test Kubernetes cluster name")
	cmd.Flags().BoolVar(&flags.logToFile, "log-to-file", false, "Specifies if should log test output to file. Disabled by default.")
	cmd.Flags().BoolVar(&flags.ignoreWebhookFailures, "ignore-webhook-failures", false, "Specifies if webhook errors should be ignored. Useful when running test locally. False by default")
	cmd.Flags().BoolVar(&flags.deployChaosJob, "deploy-chaos-job", false, "Deploy the chaos job")
	cmd.Flags().StringSliceVar(&flags.testEnvTags, "test-env-tags", nil, "Tags describing the environment for this test run")
	logutil.BindFlags(cmd.PersistentFlags())

	// enable setting flags via environment variables
	_ = viper.BindPFlags(cmd.Flags())

	return cmd
}

func checkWantedDirectories() error {
	wantedDirs := []string{
		filepath.Join("config", "crds"),
		filepath.Join("config", "e2e"),
	}
	for _, wantedDir := range wantedDirs {
		stat, err := os.Stat(wantedDir)
		if err != nil {
			return errors.Wrapf(err, "failed to stat directory: %s", wantedDir)
		}

		if !stat.IsDir() {
			return errors.Errorf("not a directory: %s", wantedDir)
		}
	}

	return nil
}

func randomTestRunName() string {
	rand.Seed(time.Now().UnixNano())
	letters := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	var prefix strings.Builder
	prefix.WriteString("e2e-")
	for i := 0; i < 5; i++ {
		prefix.WriteRune(letters[rand.Intn(len(letters))]) //nolint:gosec
	}

	return prefix.String()
}
