// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

// Sequence returns a list of steps corresponding to the basic workflow (some optional init steps, then init steps,
// create steps, check steps, then something and delete steps to terminate).
func Sequence(before StepsFunc, f StepsFunc, builders ...Builder) StepList {
	k := NewK8sClientOrFatal()

	steps := StepList{}

	if before != nil {
		steps = steps.WithSteps(before(k))
	}

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

	return steps
}
