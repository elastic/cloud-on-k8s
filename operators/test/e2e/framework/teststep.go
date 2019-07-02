// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package framework

import (
	"errors"
	"testing"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

// TestStep represents a single test
type TestStep struct {
	Name string
	Test func(t *testing.T)
}

// TestStepList defines a list of TestStep
type TestStepList []TestStep

// WithSteps appends the given TestStepList to the TestStepList
func (l TestStepList) WithSteps(testSteps TestStepList) TestStepList {
	return append(l, testSteps...)
}

// WithStep appends the given TestStep to the TestStepList
func (l TestStepList) WithStep(testStep TestStep) TestStepList {
	return append(l, testStep)
}

// RunSequential runs the TestStepList sequentially, and fails fast on first error.
func (l TestStepList) RunSequential(t *testing.T) {
	for _, ts := range l {
		if !t.Run(ts.Name, ts.Test) {
			logf.Log.Error(errors.New("test failure"), "stopping early")
			t.FailNow()
		}
	}
}

type TestStepsFunc func(k *K8sClient) TestStepList

func EmptySteps(_ *K8sClient) TestStepList {
	return TestStepList{}
}
