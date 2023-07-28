// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/elastic/cloud-on-k8s/v2/hack/upgrade-test-harness/config"
	"github.com/elastic/cloud-on-k8s/v2/hack/upgrade-test-harness/fixture"
)

type configOpts struct {
	confFile                string
	fromRelease             string
	logLevel                string
	retryCount              uint
	retryDelay              time.Duration
	retryTimeout            time.Duration
	skipCleanup             bool
	toRelease               string
	upcomingReleaseOperator string
	upcomingReleaseCRDs     string
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
		Version:       "0.2.0",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          doRun,
	}

	cmd.Flags().StringVar(&opts.confFile, "conf-file", "conf.yaml", "Path to the file containing test params")
	cmd.Flags().StringVar(&opts.fromRelease, "from-release", "v170", "Release to start with (a directory with this value must exist in testdata/)")
	cmd.Flags().StringVar(&opts.logLevel, "log-level", "INFO", "Log level (DEBUG, INFO, WARN, ERROR)")
	cmd.Flags().UintVar(&opts.retryCount, "retry-count", 60, "Number of retries")
	cmd.Flags().DurationVar(&opts.retryDelay, "retry-delay", 5*time.Second, "Delay between retries")
	cmd.Flags().DurationVar(&opts.retryTimeout, "retry-timeout", 15*time.Minute, "Time limit for retries")
	cmd.Flags().BoolVar(&opts.skipCleanup, "skip-cleanup", false, "Skip cleaning up after test run")
	cmd.Flags().StringVar(&opts.toRelease, "to-release", "upcoming", "Release to finish with (a directory with this value must exist in testdata/)")
	cmd.Flags().StringVar(&opts.upcomingReleaseCRDs, "upcoming-release-crds", "../../config/crds.yaml", "YAML file for installing the CRDs for the upcoming release")
	cmd.Flags().StringVar(&opts.upcomingReleaseOperator, "upcoming-release-operator", "../../config/operator.yaml", "YAML file for installing the operator for the upcoming release")

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
		if err := setupUpcomingRelease(opts.upcomingReleaseCRDs, "crds"); err != nil {
			return fmt.Errorf("failed to setup upcoming release: %w", err)
		}
		if err := setupUpcomingRelease(opts.upcomingReleaseOperator, "install"); err != nil {
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

		fixtures, err := buildUpgradeFixtures(prevTestParam, currTestParam)
		if err != nil {
			return err
		}

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

func setupUpcomingRelease(installYAML, targetYAML string) error {
	in, err := os.Open(installYAML)
	if err != nil {
		return fmt.Errorf("failed to open %s for reading: %w", installYAML, err)
	}

	defer in.Close()

	outFile := fmt.Sprintf("testdata/upcoming/%s.yaml", targetYAML)

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

func buildUpgradeFixtures(from *fixture.TestParam, to fixture.TestParam) ([]*fixture.Fixture, error) {
	isUpgrade := from != nil
	fixtures := []*fixture.Fixture{fixture.TestInstallOperator(to, isUpgrade)}

	if isUpgrade {
		testStatusOfResources, err := fixture.TestStatusOfResources(*from)
		if err != nil {
			return nil, err
		}
		fixtures = append(fixtures, testStatusOfResources)
	}

	testStatusOfResources, err := fixture.TestStatusOfResources(to)
	if err != nil {
		return nil, err
	}
	fixtures = append(fixtures, fixture.TestDeployResources(to), testStatusOfResources)

	return fixtures, nil
}
