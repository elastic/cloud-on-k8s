// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/elastic/cloud-on-k8s/hack/upgrade-test-harness/config"
	"github.com/elastic/cloud-on-k8s/hack/upgrade-test-harness/fixture"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

type configOpts struct {
	confFile            string
	fromRelease         string
	logLevel            string
	retryCount          uint
	retryDelay          time.Duration
	retryTimeout        time.Duration
	skipCleanup         bool
	toRelease           string
	upcomingReleaseYAML string
}

var (
	errInvalidVersionRange = errors.New("to-release must be higher than from-release")
	kubeConfFlags          = genericclioptions.NewConfigFlags(true)
	opts                   = configOpts{}
)

func main() {
	cmd := &cobra.Command{
		Use:           "eck-upgrade-harness",
		Short:         "Test harness for testing ECK release upgrades",
		Version:       "0.1.0",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          doRun,
	}

	cmd.Flags().StringVar(&opts.confFile, "conf-file", "conf.yaml", "Path to the file containing test params")
	cmd.Flags().StringVar(&opts.fromRelease, "from-release", "alpha", "Release to start with (alpha, beta, v101, v112, upcoming)")
	cmd.Flags().StringVar(&opts.logLevel, "log-level", "INFO", "Log level (DEBUG, INFO, WARN, ERROR)")
	cmd.Flags().UintVar(&opts.retryCount, "retry-count", 60, "Number of retries")
	cmd.Flags().DurationVar(&opts.retryDelay, "retry-delay", 5*time.Second, "Delay between retries")
	cmd.Flags().DurationVar(&opts.retryTimeout, "retry-timeout", 300*time.Second, "Time limit for retries")
	cmd.Flags().BoolVar(&opts.skipCleanup, "skip-cleanup", false, "Skip cleaning up after test run")
	cmd.Flags().StringVar(&opts.toRelease, "to-release", "upcoming", "Release to finish with (alpha, beta, v101, v112, upcoming)")
	cmd.Flags().StringVar(&opts.upcomingReleaseYAML, "upcoming-release-yaml", "../../config/all-in-one.yaml", "YAML file for installing the upcoming release")

	kubeConfFlags.AddFlags(cmd.Flags())

	if err := cmd.Execute(); err != nil {
		zap.S().Errorw("Test failed", "error", err)
		os.Exit(1)
	}
}

func doRun(_ *cobra.Command, _ []string) error {
	config.InitLogging(opts.logLevel)

	// load test configurations
	conf, err := config.Load(opts.confFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// determine the release to test
	from := mustGetReleasePos(conf, opts.fromRelease)
	to := mustGetReleasePos(conf, opts.toRelease)

	if to <= from {
		return errInvalidVersionRange
	}

	// setup upcoming release if necessary
	if conf.TestParams[to].Name == "upcoming" {
		if err := setupUpcomingRelease(opts.upcomingReleaseYAML); err != nil {
			return fmt.Errorf("failed to setup upcoming release: %w", err)
		}
	}

	// create test context
	ctx, err := fixture.NewTestContext(kubeConfFlags, int(opts.retryCount), opts.retryDelay, opts.retryTimeout)
	if err != nil {
		return err
	}

	// configure cleanup
	if !opts.skipCleanup {
		defer func() {
			ctx.Info("Starting cleanup")

			if err := ctx.Cleanup(ctx); err != nil {
				ctx.Errorw("Failed to cleanup", "error", err)
			}
		}()
	}

	// run upgrade fixtures
	var prevTestParam *fixture.TestParam

	for i := from; i <= to; i++ {
		currTestParam := conf.TestParams[i]

		fixtures := buildUpgradeFixtures(prevTestParam, currTestParam)

		ctx.Infof("=====[%s]=====", currTestParam.Name)

		for _, f := range fixtures {
			if err := f.Execute(ctx); err != nil {
				return err
			}
		}

		prevTestParam = &currTestParam
	}

	// try to scale the last deployed release
	if prevTestParam != nil {
		return fixture.TestScaleElasticsearch(*prevTestParam, 5).Execute(ctx)
	}

	return nil
}

func mustGetReleasePos(conf *config.File, name string) int {
	pos, err := conf.ReleasePos(name)
	if err != nil {
		panic(err)
	}

	return pos
}

func setupUpcomingRelease(installYAML string) error {
	in, err := os.Open(installYAML)
	if err != nil {
		return fmt.Errorf("failed to open %s for reading: %w", installYAML, err)
	}

	defer in.Close()

	outFile := "testdata/upcoming/install.yaml"

	out, err := os.OpenFile(outFile, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0755)
	if err != nil {
		return fmt.Errorf("failed to open %s for writing: %w", outFile, err)
	}

	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", installYAML, outFile, err)
	}

	return nil
}

func buildUpgradeFixtures(from *fixture.TestParam, to fixture.TestParam) []*fixture.Fixture {
	fixtures := []*fixture.Fixture{fixture.TestInstallOperator(to)}

	if from != nil {
		fixtures = append(fixtures, fixture.TestStatusOfResources(*from))

		// upgrade from alpha requires deleting the finalizers
		if from.Name == "alpha" {
			fixtures = append(fixtures, fixture.TestRemoveFinalizers(*from))
			// delete the stack as alpha resources are no longer reconciled by later versions of the operator
			fixtures = append(fixtures, fixture.TestRemoveResources(*from))
		}
	}

	fixtures = append(fixtures, fixture.TestDeployResources(to), fixture.TestStatusOfResources(to))

	return fixtures
}
