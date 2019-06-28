// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package framework

import (
	"testing"
)

// RunMutationsTests tests resources changes on given resources.
// If the resource to mutate to is the same as the original resource, then all tests should still pass.
func RunMutationsTests(t *testing.T, creationBuilders []Builder, mutationBuilders []Builder) {
	k := NewK8sClientOrFatal()
	steps := TestStepList{}

	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(toCreate.InitTestSteps(k))
	}
	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(toCreate.CreationTestSteps(k))
	}
	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(CheckTestSteps(toCreate, k))
	}

	// Trigger some mutations
	for _, mutateTo := range mutationBuilders {
		steps = steps.WithSteps(mutateTo.MutationTestSteps(k))
	}

	for _, mutateTo := range mutationBuilders {
		steps = steps.WithSteps(mutateTo.DeletionTestSteps(k))
	}

	steps.RunSequential(t)
}

func RunMutationTests(t *testing.T, toCreate Builder, mutateTo Builder) {
	RunMutationsTests(t, []Builder{toCreate}, []Builder{mutateTo})
}
