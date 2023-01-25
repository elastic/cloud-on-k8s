// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

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

// RunSequential runs the StepList sequentially, and fails fast on first error.
//
//nolint:thelper
func (l StepList) RunSequential(t *testing.T) {
	for _, ts := range l {
		if ts.Skip != nil && ts.Skip() {
			log.Info("Skipping test", "name", ts.Name)
			continue
		}
		if !t.Run(ts.Name, ts.Test) {
			logf.Log.Error(errors.New("test failure"), "running diagnostics and continuing")
			if ts.OnFailure != nil {
				ts.OnFailure()
			}
			// Where is it writing this?
			// How do you upload this to google cloud storage?
			runECKDiagnostics()
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
		log.Error(err, "Failed to run eck-diagnostics")
	}
}

type StepsFunc func(k *K8sClient) StepList

func EmptySteps(_ *K8sClient) StepList {
	return StepList{}
}
