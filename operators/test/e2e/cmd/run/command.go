// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package run

import (
	"math/rand"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

type runFlags struct {
	managedNamespaces   []string
	e2eImage            string
	elasticStackVersion string
	kubeConfig          string
	operatorImage       string
	testContextOutPath  string
	testLicence         string
	scratchDirRoot      string
	testRegex           string
	testRunName         string
	commandTimeout      time.Duration
	autoPortForwarding  bool
	skipCleanup         bool
	local               bool
}

var log logr.Logger

func Command() *cobra.Command {
	flags := runFlags{}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "setup an e2e test environment and run tests",
		PreRun: func(_ *cobra.Command, _ []string) {
			log = logf.Log.WithName(flags.testRunName)
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			return doRun(flags)
		},
	}

	cmd.Flags().BoolVar(&flags.autoPortForwarding, "auto-port-forwarding", false, "Enable port forwarding to pods")
	cmd.Flags().DurationVar(&flags.commandTimeout, "command-timeout", 90*time.Second, "Timeout for commands executed")
	cmd.Flags().StringVar(&flags.e2eImage, "e2e-image", "", "E2E test image")
	cmd.Flags().StringVar(&flags.elasticStackVersion, "elastic-stack-version", "7.1.1", "Elastic stack version")
	cmd.Flags().StringVar(&flags.kubeConfig, "kubeconfig", "", "Path to kubeconfig")
	cmd.Flags().BoolVar(&flags.local, "local", false, "Create the environment for running tests locally")
	cmd.Flags().StringSliceVar(&flags.managedNamespaces, "managed-namespaces", []string{"mercury", "venus"}, "List of managed namespaces")
	cmd.Flags().StringVar(&flags.operatorImage, "operator-image", "", "Operator image")
	cmd.Flags().BoolVar(&flags.skipCleanup, "skip-cleanup", false, "Do not run cleanup actions after test run")
	cmd.Flags().StringVar(&flags.testContextOutPath, "test-context-out", "", "Write the test context to the given path")
	cmd.Flags().StringVar(&flags.testLicence, "test-licence", "", "Test licence to apply")
	cmd.Flags().StringVar(&flags.scratchDirRoot, "scratch-dir", "/tmp/eck-e2e", "Path under which temporary files should be created")
	cmd.Flags().StringVar(&flags.testRegex, "test-regex", "", "Regex to pass to the test runner")
	cmd.Flags().StringVar(&flags.testRunName, "test-run-name", randomTestRunName(), "Name of this test run")

	// enable setting flags via environment variables
	_ = viper.BindPFlags(cmd.Flags())

	return cmd
}

func randomTestRunName() string {
	rand.Seed(time.Now().UnixNano())
	letters := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	var prefix strings.Builder
	prefix.WriteString("e2e-")
	for i := 0; i < 5; i++ {
		prefix.WriteRune(letters[rand.Intn(len(letters))])
	}

	return prefix.String()
}
