// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"testing"
)

type ReversalTestState interface {
	PreMutationSteps(k *K8sClient) StepList
	PostMutationSteps(k *K8sClient) StepList
	VerificationSteps(k *K8sClient) StepList
}

// RunMutationReversal tests mutations that are either invalid or aborted mid way leading to a configuration reversal of
// the original configuration.
func RunMutationReversal(t *testing.T, creationBuilders []Builder, mutationBuilders []Builder, state ReversalTestState) {
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
	// set up the mutation test
	steps = steps.WithSteps(state.PreMutationSteps(k))

	// trigger some mutations
	for _, mutateTo := range mutationBuilders {
		steps = steps.WithSteps(mutateTo.UpgradeTestSteps(k))
	}

	// ensure the desired progress has been made with the mutation
	steps = steps.WithSteps(state.PostMutationSteps(k))

	// now revert the mutation
	for _, toRevertTo := range creationBuilders {
		steps = steps.WithSteps(toRevertTo.UpgradeTestSteps(k))
	}

	// run the standard checks once more
	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(CheckTestSteps(toCreate, k))
	}

	// verify the specifics of the upgrade reversal
	steps = steps.WithSteps(state.VerificationSteps(k))

	// and delete the resources
	for _, mutateTo := range mutationBuilders {
		steps = steps.WithSteps(mutateTo.DeletionTestSteps(k))
	}

	steps.RunSequential(t)
}
