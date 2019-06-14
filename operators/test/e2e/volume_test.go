/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package e2e

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/stack"
)

func TestVolumeDefaultPVC(t *testing.T) {
	k := helpers.NewK8sClientOrFatal()

	initStack := stack.NewStackBuilder("test-es-default-pvc").
		WithESMasterNodes(1, stack.DefaultResources)

	helpers.TestStepList{}.
		WithSteps(stack.InitTestSteps(initStack, k)...).
		WithSteps(stack.CreationTestSteps(initStack, k)...).
		WithSteps(stack.ESClusterChecks(initStack.Elasticsearch, k)...).
		WithSteps(stack.CheckDefaultPVC(initStack.Elasticsearch, k)).
		WithSteps(stack.DeletionTestSteps(initStack, k)...).
		RunSequential(t)

}

func TestVolumeEmptyDir(t *testing.T) {
	k := helpers.NewK8sClientOrFatal()

	initStack := stack.NewStackBuilder("test-es-explicit-empty-dir").
		WithESMasterNodes(1, stack.DefaultResources).
		WithEmptyDirVolumes()

	helpers.TestStepList{}.
		WithSteps(stack.InitTestSteps(initStack, k)...).
		WithSteps(stack.CreationTestSteps(initStack, k)...).
		WithSteps(stack.ESClusterChecks(initStack.Elasticsearch, k)...).
		WithSteps(stack.CheckEmptyDir(initStack.Elasticsearch, k)).
		WithSteps(stack.DeletionTestSteps(initStack, k)...).
		RunSequential(t)
}
