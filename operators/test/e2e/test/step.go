// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"errors"
	"testing"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

// Step represents a single test
type Step struct {
	Name string
	Test func(t *testing.T)
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
func (l StepList) RunSequential(t *testing.T) {
	for _, ts := range l {
		if !t.Run(ts.Name, ts.Test) {
			logf.Log.Error(errors.New("test failure"), "stopping early")
			t.FailNow()
		}
	}
}

type StepsFunc func(k *K8sClient) StepList

func EmptySteps(_ *K8sClient) StepList {
	return StepList{}
}
