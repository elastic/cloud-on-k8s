// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"testing"
)

type MutationOptions struct {
	// IncludesRollingUpgrade specifies if the mutation to perform includes some rolling upgrade,
	// for which shards with 0 replicas are expected to become unavailable.
	IncludesRollingUpgrade bool
}

// RunMutations tests resources changes on given resources.
// If the resource to mutate to is the same as the original resource, then all tests should still pass.
func RunMutations(t *testing.T, creationBuilders []Builder, mutationBuilders []Builder, opts MutationOptions) {
	k := NewK8sClientOrFatal()
	steps := StepList{}

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
		steps = steps.WithSteps(mutateTo.MutationTestSteps(k, opts))
	}

	for _, mutateTo := range mutationBuilders {
		steps = steps.WithSteps(mutateTo.DeletionTestSteps(k))
	}

	steps.RunSequential(t)
}

// RunMutations tests one resource change on a given resource.
func RunMutation(t *testing.T, toCreate Builder, mutateTo Builder, options MutationOptions) {
	RunMutations(t, []Builder{toCreate}, []Builder{mutateTo}, options)
}
