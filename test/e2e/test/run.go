// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import "testing"

func shouldSkipTest(builders ...Builder) bool {
	for _, b := range builders {
		// ignore the test if some builders cannot be tested
		if b.SkipTest() {
			return true
		}
	}
	return false
}

func skipIfIncompatibleBuilders(t *testing.T, builders ...Builder) {
	t.Helper()
	if shouldSkipTest(builders...) {
		t.Skip("Skipping test due to an incompatible builder")
	}
}

// Sequence returns a list of steps corresponding to the basic workflow (some optional init steps, then init steps,
// create steps, check steps, then something and delete steps to terminate).
func Sequence(before StepsFunc, f StepsFunc, builders ...Builder) StepList {
	steps := StepList{}
	if shouldSkipTest(builders...) {
		return steps
	}

	k := NewK8sClientOrFatal()

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

// BeforeAfterSequence returns a list of steps corresponding to a workflow that allows defining a list of steps to execute
// before and after builder workflow (before steps, init, create, checks, deletes, after steps)
func BeforeAfterSequence(before StepsFunc, after StepsFunc, builders ...Builder) StepList {
	steps := StepList{}
	if shouldSkipTest(builders...) {
		return steps
	}

	k := NewK8sClientOrFatal()

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
	for _, b := range builders {
		steps = steps.WithSteps(b.DeletionTestSteps(k))
	}

	if after != nil {
		steps = steps.WithSteps(after(k))
	}

	return steps
}
