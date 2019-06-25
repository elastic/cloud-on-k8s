// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
)

type Builder interface {
	// InitTestSteps includes pre-requisite tests (eg. is k8s accessible),
	// and cleanup from previous tests.
	InitTestSteps(k *helpers.K8sHelper) helpers.TestStepList
	// CreationTestSteps returns all test steps to create a given stack.
	CreationTestSteps(k *helpers.K8sHelper) helpers.TestStepList
	// CheckStackSteps returns all test steps to verify the status of the given stack
	CheckStackSteps(k *helpers.K8sHelper) helpers.TestStepList
	// DeletionTestSteps returns all test step to delete a given stack
	DeletionTestSteps(k *helpers.K8sHelper) helpers.TestStepList
	// MutationTestSteps returns all test steps to test topology changes on the given stack.
	// We expect the stack to be already created and running.
	// If the stack to mutate to is the same as the original stack,
	// then all tests should still pass.
	MutationTestSteps(k *helpers.K8sHelper) helpers.TestStepList
}
