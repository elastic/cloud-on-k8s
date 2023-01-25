// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/command"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Step represents a single test
type Step struct {
	Name      string
	Test      func(t *testing.T)
	Skip      func() bool // returns true if the test should be skipped
	OnFailure func()      // additional step specific callback on failure
}

// StepList defines a list of Step
type StepList []Step

// WithSteps appends the given StepList to the StepList
func (l StepList) WithSteps(testSteps StepList) StepList {
	return append(l, testSteps...)
}

// WithStep appends the given Step to the StepList
func (l StepList) WithStep(testStep Step) StepList {
	return append(l, testStep)
}

// RunSequential runs the StepList sequentially, continues on any errors.
// Runs eck-diagnostics and uploads artifacts when run within the CI system.
//
//nolint:thelper
func (l StepList) RunSequential(t *testing.T) {
	for _, ts := range l {
		if ts.Skip != nil && ts.Skip() {
			log.Info("Skipping test", "name", ts.Name)
			continue
		}
		if !t.Run(ts.Name, ts.Test) {
			logf.Log.Error(errors.New("test failure"), "continuing with additional tests")
			if ts.OnFailure != nil {
				ts.OnFailure()
			}
			// Only run eck diagnostics after each run when job is
			// run from CI, which provides a 'job-name' flag.
			if Ctx().JobName != "" {
				logf.Log.Info("running eck-diagnostics")
				runECKDiagnostics()
				logf.Log.Info("uploading artifacts from diagnostics")
				uploadDiagnosticsArtifacts()
			}
			command.NewKubectl("")
		}
	}
}

func runECKDiagnostics() {
	ctx := Ctx()
	operatorNS := ctx.Operator.Namespace
	otherNS := append([]string{ctx.E2ENamespace}, ctx.Operator.ManagedNamespaces...)
	cmd := exec.Command("eck-diagnostics", "-o", operatorNS, "-r", strings.Join(otherNS, ","), "--run-agent-diagnostics")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Error(err, fmt.Sprintf("Failed to run eck-diagnostics: %s", err))
	}
}

func uploadDiagnosticsArtifacts() {
	ctx := Ctx()
	cmd := exec.Command("gsutil", "cp", "*.zip", fmt.Sprintf("gs://devops-ci-artifacts/jobs/%s/%s/", ctx.JobName, ctx.BuildNumber))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Error(err, fmt.Sprintf("Failed to run gsutil: %s", err))
	}
}

type StepsFunc func(k *K8sClient) StepList

func EmptySteps(_ *K8sClient) StepList {
	return StepList{}
}
