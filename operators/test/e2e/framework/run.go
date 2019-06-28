// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package framework

import "testing"

// Run tests something on given resources.
func Run(t *testing.T, f TestStepsFunc, builders ...Builder) {
	k := NewK8sClientOrFatal()

	steps := TestStepList{}

	for _, b := range builders {
		steps = steps.WithSteps(b.InitTestSteps(k))
	}
	for _, b := range builders {
		steps = steps.WithSteps(b.CreationTestSteps(k))
	}
	for _, b := range builders {
		steps = steps.WithSteps(CheckTestSteps(b, k))
	}

	// Trigger something
	steps = steps.WithSteps(f(k))

	for _, b := range builders {
		steps = steps.WithSteps(b.DeletionTestSteps(k))
	}

	steps.RunSequential(t)
}
