// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

// BuilderHashAnnotation is the name of an annotation set by the E2E tests on resources containing the hash of their
// Builder for comparison purposes (pre/post rolling upgrade).
const BuilderHashAnnotation = "k8s.elastic.co/e2e-builder-hash"

type Builder interface {
	// InitTestSteps includes pre-requisite tests (eg. is k8s accessible) and cleanup from previous tests.
	InitTestSteps(k *K8sClient) StepList
	// CreationTestSteps returns all test steps to create a resource. (The resource is not deleted at the end.)
	CreationTestSteps(k *K8sClient) StepList
	// CheckK8sTestSteps returns all test steps to verify the given resource in K8s is the expected one.
	CheckK8sTestSteps(k *K8sClient) StepList
	// CheckStackTestSteps returns all test steps to verify the given resource is running as expected
	CheckStackTestSteps(k *K8sClient) StepList
	// UpgradeTestSteps returns all the steps necessary to upgrade an existing resource to new revision.
	UpgradeTestSteps(k *K8sClient) StepList
	// DeletionTestSteps returns all test step to delete a resource.
	DeletionTestSteps(k *K8sClient) StepList
	// MutationTestSteps returns all test steps to test changes on a resource.
	// We expect the resource to be already created and running.
	// If the resource to mutate to is the same as the original resource, then all tests should still pass.
	MutationTestSteps(k *K8sClient) StepList
	// MutationReversalTestContext returns a context struct to test changes on a resource that are immediately reverted.
	// We assume the resource to be ready and running.
	// We assume the resource to be the same as the original resource after reversion.
	MutationReversalTestContext() ReversalTestContext
	// Skip the test if true.
	SkipTest() bool
}
