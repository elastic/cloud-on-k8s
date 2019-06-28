// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework/elasticsearch"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// TestMdiToDedicatedMutation creates a 1 master + data cluster,
// then mutates it to 1 dedicated master + 1 dedicated data cluster
func TestMutationMdiToDedicated(t *testing.T) {
	// create a 1 md node cluster
	b := elasticsearch.NewBuilder("test-mutation-mdi-to-dedicated").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	// mutate to 1 m node + 1 d node
	mutated := b.
		WithNoESTopology().
		WithESDataNodes(1, elasticsearch.DefaultResources).
		WithESMasterNodes(1, elasticsearch.DefaultResources)

	framework.RunMutationTests(t, b, mutated)
}

// TestMutationMoreNodes creates a 1 node cluster,
// then mutates it to a 3 nodes cluster
func TestMutationMoreNodes(t *testing.T) {
	// create an ES cluster with 1 node
	b := elasticsearch.NewBuilder("test-mutation-more-nodes").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	// mutate it to 2 nodes
	mutated := b.
		WithNoESTopology().
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	framework.RunMutationTests(t, b, mutated)
}

// TestMutationLessNodes creates a 3 node cluster,
// then mutates it to a 1 node cluster
func TestMutationLessNodes(t *testing.T) {
	// create an ES cluster with 3 node
	b := elasticsearch.NewBuilder("test-mutation-less-nodes").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)
	// mutate it to 1 node
	mutated := b.
		WithNoESTopology().
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	framework.RunMutationTests(t, b, mutated)
}

// TestMutationResizeMemoryUp creates a 1 node cluster,
// then mutates it to a 1 node cluster with more RAM
func TestMutationResizeMemoryUp(t *testing.T) {
	// create an ES cluster with a 2G node
	b := elasticsearch.NewBuilder("test-mutation-resize-memory-up").
		WithESMasterDataNodes(1, corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("2Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
		})
	// mutate it to 1 node with 4G memory
	mutated := b.
		WithNoESTopology().
		WithESMasterDataNodes(1, corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("4Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
		})

	framework.RunMutationTests(t, b, mutated)
}

// TestMutationResizeMemoryDown creates a 1 node cluster,
// then mutates it to a 1 node cluster with less RAM
func TestMutationResizeMemoryDown(t *testing.T) {
	// create an ES cluster with a 4G node
	b := elasticsearch.NewBuilder("test-mutation-resize-memory-down").
		WithESMasterDataNodes(1, corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("4Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
		})
	// mutate it to 1 node with 2G memory
	mutated := b.
		WithNoESTopology().
		WithESMasterDataNodes(1, corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("2Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
		})

	framework.RunMutationTests(t, b, mutated)
}
