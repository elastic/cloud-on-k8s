// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
)

// RunCreationMutationDeletionTests does exactly what its names is suggesting :)
// If the stack we mutate to is the same as the original stack, tests should still pass.
func RunCreationMutationDeletionTests(t *testing.T, toCreate v1alpha1.Stack, mutateTo v1alpha1.Stack) {
	k := helpers.NewK8sClientOrFatal()
	helpers.TestStepList{}.
		WithSteps(InitTestSteps(toCreate, k)...).
		WithSteps(CreationTestSteps(toCreate, k)...).
		WithSteps(MutationTestSteps(mutateTo, k)...).
		WithSteps(DeletionTestSteps(mutateTo, k)...).
		RunSequential(t)
}
