// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// TestMutationHTTPToHTTPS creates a 3 node cluster running without TLS on the HTTP layer,
// then mutates it to a 3 node cluster running with TLS.
func TestMutationHTTPToHTTPS(t *testing.T) {
	// create a 3 md node cluster
	b := elasticsearch.NewBuilder("test-mutation-http-to-https").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithTLSDisabled(true)

	// mutate to https
	mutated := b.WithTLSDisabled(false)

	test.RunMutation(t, b, mutated)
}

// TestMutationHTTPSToHTTP creates a 3 node cluster
// then mutates it to a 3 node cluster running without TLS on the HTTP layer.
func TestMutationHTTPSToHTTP(t *testing.T) {
	// create a 3 md node cluster
	b := elasticsearch.NewBuilder("test-mutation-http-to-https").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	// mutate to http
	mutated := b.WithTLSDisabled(true)

	test.RunMutation(t, b, mutated)
}

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

	RunESMutation(t, b, mutated)
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

	RunESMutation(t, b, mutated)
}

// TestMutationLessNodes creates a 3 node cluster,
// then mutates it to a 1 node cluster.
// Covers the special case of going from 2 to 1 master node with zen1.
func TestMutationLessNodes(t *testing.T) {
	// create an ES cluster with 3 node
	b := elasticsearch.NewBuilder("test-mutation-less-nodes").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)
	// mutate it to 1 node
	mutated := b.
		WithNoESTopology().
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	RunESMutation(t, b, mutated)
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

	RunESMutation(t, b, mutated)
}

// TestMutationResizeMemoryDown creates a 1 node cluster,
// then mutates it to a 1 node cluster with less RAM
func TestMutationResizeMemoryDown(t *testing.T) {
	// create an ES cluster with a 4G node
	b := elasticsearch.NewBuilder("test-mutation-resize-mem-down").
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

	RunESMutation(t, b, mutated)
}

// TestMutationSecondMasterSet add a separate set of dedicated masters
// to an existing cluster.
func TestMutationSecondMasterSet(t *testing.T) {
	b := elasticsearch.NewBuilder("test-mutation-2nd-master-set").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	// add a second master sset
	mutated := b.WithNoESTopology().
		WithESMasterDataNodes(2, elasticsearch.DefaultResources).
		WithESMasterNodes(3, elasticsearch.DefaultResources)

	RunESMutation(t, b, mutated)
}

// TestMutationSecondMasterSetDown test a downscale of a separate set of dedicated masters.
func TestMutationSecondMasterSetDown(t *testing.T) {
	b := elasticsearch.NewBuilder("test-mutation-2nd-master-set").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources).
		WithESMasterNodes(3, elasticsearch.DefaultResources)

	// scale down to single node
	mutated := b.WithNoESTopology().
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	RunESMutation(t, b, mutated)
}

// TestVersionUpgrade680To720 creates a cluster in version 6.8.0,
// and upgrades it to 7.2.0.
func TestVersionUpgrade680To720(t *testing.T) {
	// create an ES cluster with 3 x 6.8.0 nodes
	initial := elasticsearch.NewBuilder("test-version-up-680-to-720").
		WithVersion("6.8.0").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)
	// mutate it to 3 x 7.2.0 nodes
	mutated := initial.WithNoESTopology().
		WithVersion("7.2.0").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func RunESMutation(t *testing.T, toCreate elasticsearch.Builder, mutateTo elasticsearch.Builder) {
	mutateTo.MutatedFrom = &toCreate
	test.RunMutation(t, toCreate, mutateTo)
}
