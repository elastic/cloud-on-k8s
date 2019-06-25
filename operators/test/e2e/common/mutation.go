// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
)

// RunCreationMutationDeletionTests does exactly what its names is suggesting :)
// If the stack we mutate to is the same as the original stack, tests should still pass.
func RunMutationsTests(t *testing.T, creationBuilders []Builder, mutationBuilders []Builder) {
	k := helpers.NewK8sClientOrFatal()
	steps := helpers.TestStepList{}

	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(toCreate.InitTestSteps(k))
	}
	for _, toCreate := range creationBuilders {
		steps = steps.WithSteps(toCreate.CreationTestSteps(k))
	}
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
