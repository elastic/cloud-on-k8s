// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
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

	RunESMutation(t, b, mutated)
}

// TestMutationHTTPSToHTTP creates a 3 node cluster
// then mutates it to a 3 node cluster running without TLS on the HTTP layer.
func TestMutationHTTPSToHTTP(t *testing.T) {
	// create a 3 md node cluster
	b := elasticsearch.NewBuilder("test-mutation-https-to-http").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	// mutate to http
	mutated := b.WithTLSDisabled(true)

	RunESMutation(t, b, mutated)
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

// TestMutationResizeMemoryUp creates a 3 node cluster,
// then mutates it to a 3 nodes cluster with more RAM.
func TestMutationResizeMemoryUp(t *testing.T) {
	// create an ES cluster with 3 x 2G nodes
	b := elasticsearch.NewBuilder("test-mutation-resize-memory-up").
		WithESMasterDataNodes(3, corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("2Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
		})
	// mutate it to 3 x 4GB nodes
	mutated := b.
		WithNoESTopology().
		WithESMasterDataNodes(3, corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("4Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
		})

	RunESMutation(t, b, mutated)
}

// TestMutationResizeMemoryDown creates a 3 nodes cluster,
// then mutates it to a 3 nodes cluster with less RAM.
func TestMutationResizeMemoryDown(t *testing.T) {
	// create an ES cluster with 3 x 4G nodes
	b := elasticsearch.NewBuilder("test-mutation-resize-mem-down").
		WithESMasterDataNodes(3, corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("4Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
		})
	// mutate it to 3 x 2GB nodes
	mutated := b.
		WithNoESTopology().
		WithESMasterDataNodes(3, corev1.ResourceRequirements{
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

	// added to debug https://github.com/elastic/cloud-on-k8s/issues/5865 can be removed once stable
	if version.MustParse(b.Elasticsearch.Spec.Version).GTE(version.MinFor(7, 7, 0)) {
		b = b.WithAdditionalConfig(map[string]map[string]interface{}{
			"masterdata": {
				"logger.org.elasticsearch.http.HttpTracer": "TRACE",
				"http.tracer.include":                      []string{"*_cluster/health*"},
			},
			"master": {
				"logger.org.elasticsearch.http.HttpTracer": "TRACE",
				"http.tracer.include":                      []string{"*_cluster/health*"},
			},
		})

		mutated = mutated.WithAdditionalConfig(map[string]map[string]interface{}{
			"masterdata": {
				"logger.org.elasticsearch.http.HttpTracer": "TRACE",
				"http.tracer.include":                      []string{"*_cluster/health*"},
			},
		})
	}
	RunESMutation(t, b, mutated)
}

// TestMutationRollingDownscaleCombination combines a rolling update with scale down operation.
func TestMutationRollingDownscaleCombination(t *testing.T) {
	b := elasticsearch.NewBuilder("test-combined-upgrade-downscale").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(2, "data-1", elasticsearch.DefaultResources).
		WithNamedESDataNodes(2, "data-2", elasticsearch.DefaultResources)

	mutated := b.WithNoESTopology().
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(2, "data-1", elasticsearch.DefaultResources).
		WithNamedESDataNodes(1, "data-2", elasticsearch.DefaultResources). // scaling down data-2
		WithAdditionalConfig(map[string]map[string]interface{}{
			"data-1": {
				"node.attr.important": "attribute", // triggers the rolling update on data-1
			},
		})
	RunESMutation(t, b, mutated)
}

func TestMutationAndReversal(t *testing.T) {
	b := elasticsearch.NewBuilder("test-reverted-mutation").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	mutation := b.DeepCopy().
		WithAdditionalConfig(map[string]map[string]interface{}{
			"masterdata": {
				"node.attr.box_type": "mixed",
			},
		}).
		WithMutatedFrom(&b)

	reversed := b.DeepCopy().WithMutatedFrom(&mutation)

	test.RunMutations(t, []test.Builder{b}, []test.Builder{mutation, reversed})

}

func TestMutationNodeSetReplacementWithChangeBudget(t *testing.T) {
	b := elasticsearch.NewBuilder("test-1-change-budget").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(5, "data1", elasticsearch.DefaultResources)

	// rename data set from data1 to data2, add change budget
	mutated := b.WithNoESTopology().
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(5, "data2", elasticsearch.DefaultResources).
		WithChangeBudget(1, 1)

	RunESMutation(t, b, mutated)
}

func TestMutationWithLargerMaxUnavailable(t *testing.T) {
	b := elasticsearch.NewBuilder("test-2-change-budget").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(2, "data1", elasticsearch.DefaultResources)

	// trigger a mutation that will lead to a rolling upgrade
	mutated := b.WithNoESTopology().
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(2, "data1", elasticsearch.DefaultResources).
		WithAdditionalConfig(map[string]map[string]interface{}{
			"data1": {
				"node.attr.value": "this-is-fine",
			},
		}).
		WithChangeBudget(1, 2)

	RunESMutation(t, b, mutated)
}

func RunESMutation(t *testing.T, toCreate elasticsearch.Builder, mutateTo elasticsearch.Builder) {
	test.RunMutation(t, toCreate, mutateTo.WithMutatedFrom(&toCreate))
}
