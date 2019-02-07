// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helpers

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

// WithSteps appends the given TestSteps to the TestStepList
func (l TestStepList) WithSteps(testSteps ...TestStep) TestStepList {
	return append(l, testSteps...)
}

// RunSequential runs the TestSteps sequentially,
// and fails fast on first error
func (l TestStepList) RunSequential(t *testing.T) {
	for _, ts := range l {
		if !t.Run(ts.Name, ts.Test) {
			logf.Log.Error(errors.New("test failure"), "Stopping early.")
			t.FailNow()
		}
	}
}
