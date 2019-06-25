// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
)

type FailureTestFunc func(k *helpers.K8sHelper) helpers.TestStepList

func RunFailureTest(t *testing.T, f FailureTestFunc, builders ...Builder) {
	k := helpers.NewK8sClientOrFatal()

	steps := helpers.TestStepList{}

	for _, b := range builders {
		steps = steps.WithSteps(b.InitTestSteps(k))
	}
	for _, b := range builders {
		steps = steps.WithSteps(b.CreationTestSteps(k))
	}

	// Trigger some kind of catastrophe
	steps = steps.WithSteps(f(k))

	// Check we recover
	for _, b := range builders {
		steps = steps.WithSteps(b.CheckStackSteps(k))
	}
	for _, b := range builders {
		steps = steps.WithSteps(b.DeletionTestSteps(k))
	}

	steps.RunSequential(t)
}
