// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/common"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/elasticsearch"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// TestMdiToDedicatedMutation creates a 1 master + data cluster,
// then mutates it to 1 dedicated master + 1 dedicated data cluster
func TestMutationMdiToDedicated(t *testing.T) {
	// create a 1 md node cluster
	es := elasticsearch.NewBuilder("test-mutation-mdi-to-dedicated").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	// mutate to 1 m node + 1 d node
	mutatedEs := es.
		WithNoESTopology().
		WithESDataNodes(1, elasticsearch.DefaultResources).
		WithESMasterNodes(1, elasticsearch.DefaultResources)

	common.RunMutationTests(t, es, mutatedEs)
}

// TestMutationMoreNodes creates a 1 node cluster,
// then mutates it to a 3 nodes cluster
func TestMutationMoreNodes(t *testing.T) {
	// create a stack with 1 node
	es := elasticsearch.NewBuilder("test-mutation-more-nodes").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	// mutate it to 2 nodes
	mutatedEs := es.
		WithNoESTopology().
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	common.RunMutationTests(t, es, mutatedEs)
}

// TestMutationLessNodes creates a 3 node cluster,
// then mutates it to a 1 node cluster
func TestMutationLessNodes(t *testing.T) {
	// create a stack with 3 node
	es := elasticsearch.NewBuilder("test-mutation-less-nodes").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)
	// mutate it to 1 node
	mutatedEs := es.
		WithNoESTopology().
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	common.RunMutationTests(t, es, mutatedEs)
}

// TestMutationResizeMemoryUp creates a 1 node cluster,
// then mutates it to a 1 node cluster with more RAM
func TestMutationResizeMemoryUp(t *testing.T) {
	// create a stack with a 2G node
	es := elasticsearch.NewBuilder("test-mutation-resize-memory-up").
		WithESMasterDataNodes(1, corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("2Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
		})
	// mutate it to 1 node with 4G memory
	mutatedEs := es.
		WithNoESTopology().
		WithESMasterDataNodes(1, corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("4Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
		})

	common.RunMutationTests(t, es, mutatedEs)
}

// TestMutationResizeMemoryDown creates a 1 node cluster,
// then mutates it to a 1 node cluster with less RAM
func TestMutationResizeMemoryDown(t *testing.T) {
	// create a stack with a 4G node
	es := elasticsearch.NewBuilder("test-mutation-resize-memory-down").
		WithESMasterDataNodes(1, corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("4Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
		})
	// mutate it to 1 node with 2G memory
	mutatedEs := es.
		WithNoESTopology().
		WithESMasterDataNodes(1, corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("2Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
		})

	common.RunMutationTests(t, es, mutatedEs)
}
