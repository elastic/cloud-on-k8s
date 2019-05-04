// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"testing"

	common "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/k8s-operators/operators/test/e2e/stack"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// TestMdiToDedicatedMutation creates a 1 master + data cluster,
// then mutates it to 1 dedicated master + 1 dedicated data cluster
func TestMutationMdiToDedicated(t *testing.T) {
	// create a 1 md node cluster
	initStack := stack.NewStackBuilder("test-mutation-mdi-to-dedicated").
		WithESMasterDataNodes(1, stack.DefaultResources)

	// mutate to 1 m node + 1 d node
	mutatedStack := initStack.
		WithNoESTopology().
		WithESDataNodes(1, stack.DefaultResources).
		WithESMasterNodes(1, stack.DefaultResources)

	stack.RunCreationMutationDeletionTests(t, initStack, mutatedStack)
}

// TestMutationMoreNodes creates a 1 node cluster,
// then mutates it to a 3 nodes cluster
func TestMutationMoreNodes(t *testing.T) {
	// create a stack with 1 node
	initStack := stack.NewStackBuilder("test-mutation-more-nodes").
		WithESMasterDataNodes(1, stack.DefaultResources)
	// mutate it to 2 nodes
	mutatedStack := initStack.
		WithNoESTopology().
		WithESMasterDataNodes(2, stack.DefaultResources)

	stack.RunCreationMutationDeletionTests(t, initStack, mutatedStack)
}

// TestMutationLessNodes creates a 3 node cluster,
// then mutates it to a 1 node cluster
func TestMutationLessNodes(t *testing.T) {
	// create a stack with 3 node
	initStack := stack.NewStackBuilder("test-mutation-less-nodes").
		WithESMasterDataNodes(3, stack.DefaultResources)
	// mutate it to 1 node
	mutatedStack := initStack.
		WithNoESTopology().
		WithESMasterDataNodes(1, stack.DefaultResources)

	stack.RunCreationMutationDeletionTests(t, initStack, mutatedStack)
}

// TestMutationResizeMemoryUp creates a 1 node cluster,
// then mutates it to a 1 node cluster with more RAM
func TestMutationResizeMemoryUp(t *testing.T) {
	// create a stack with a 1G node
	initStack := stack.NewStackBuilder("test-mutation-resize-memory-up").
		WithESMasterDataNodes(1, common.ResourcesSpec{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("1G"),
				corev1.ResourceCPU:    resource.MustParse("1"),
			},
		})
	// mutate it to 1 node with 2G memory
	mutatedStack := initStack.
		WithNoESTopology().
		WithESMasterDataNodes(1, common.ResourcesSpec{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("2G"),
				corev1.ResourceCPU:    resource.MustParse("1"),
			},
		})

	stack.RunCreationMutationDeletionTests(t, initStack, mutatedStack)
}

// TestMutationResizeMemoryDown creates a 1 node cluster,
// then mutates it to a 1 node cluster with less RAM
func TestMutationResizeMemoryDown(t *testing.T) {
	// create a stack with a 2G node
	initStack := stack.NewStackBuilder("test-mutation-resize-memory-up").
		WithESMasterDataNodes(1, common.ResourcesSpec{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("2G"),
				corev1.ResourceCPU:    resource.MustParse("1"),
			},
		})
	// mutate it to 1 node with 1G memory
	mutatedStack := initStack.
		WithNoESTopology().
		WithESMasterDataNodes(1, common.ResourcesSpec{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("1G"),
				corev1.ResourceCPU:    resource.MustParse("1"),
			},
		})

	stack.RunCreationMutationDeletionTests(t, initStack, mutatedStack)
}
