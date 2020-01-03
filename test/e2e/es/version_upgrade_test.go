// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
)

func TestVersionUpgradeSingleNode68xTo73x(t *testing.T) {
	// covers the case where the existing zen1 master needs to be upgraded/restarted to a zen2 master
	initial := elasticsearch.NewBuilder("test-version-up-1-68x-to-73x").
		WithVersion("6.8.5").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion("7.3.2").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeTwoNodes68xTo73x(t *testing.T) {
	// covers the case where 2 existing zen1 masters get upgraded/restarted to zen2 masters
	// due to minimum_master_nodes=2, the cluster is unavailable while the first master is upgraded
	initial := elasticsearch.NewBuilder("test-version-up-2-68x-to-73x").
		WithVersion("6.8.5").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion("7.3.2").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgrade3Nodes68xTo73x(t *testing.T) {
	// covers the case where 3 existing zen1 masters get upgraded/restarted to zen2 masters (standard rolling upgrade)
	initial := elasticsearch.NewBuilder("test-version-up-3-68x-to-73x").
		WithVersion("6.8.5").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion("7.3.0").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeSingleMaster68xToNewNodeSet73x(t *testing.T) {
	// covers the case where the existing zen1 master get upgraded/restarted to a zen2 master
	// but the new one is specified in a different NodeSet, hence gets created before the old one is removed
	initial := elasticsearch.NewBuilder("test-version-up-68x-to-new-73x").
		WithVersion("6.8.5").
		WithNodeSet(esv1.NodeSet{
			Name:        "master68x",
			Count:       int32(1),
			PodTemplate: elasticsearch.ESPodTemplate(elasticsearch.DefaultResources),
		})

	mutated := initial.WithNoESTopology().
		WithVersion("7.3.0").
		WithNodeSet(esv1.NodeSet{
			Name:        "master73x",
			Count:       int32(1),
			PodTemplate: elasticsearch.ESPodTemplate(elasticsearch.DefaultResources),
		})

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeSingleMaster68xToMore73x(t *testing.T) {
	// covers the case where the existing zen1 master get upgraded/restarted to a zen2 master
	// but the user defines an additional zen2 master that gets created before the old one is upgraded
	initial := elasticsearch.NewBuilder("test-version-up-1-68x-more-73x").
		WithVersion("6.8.5").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion("7.3.0").
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeSingle710To730(t *testing.T) {
	initial := elasticsearch.NewBuilder("test-version-up-1-710-to-732").
		WithVersion("7.1.0").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion("7.3.2").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeSingle740To750(t *testing.T) {
	initial := elasticsearch.NewBuilder("test-version-up-1-742-to-750").
		WithVersion("7.4.2").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion("7.5.0").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}
