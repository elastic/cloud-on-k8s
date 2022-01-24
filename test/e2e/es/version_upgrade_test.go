// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e
// +build es e2e

package es

import (
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
)

func TestVersionUpgradeSingleNode68xTo7x(t *testing.T) {
	if test.Ctx().HasTag(test.ArchARMTag) {
		t.Skipf("Skipping test because Elasticsearch 6.8.x does not have an ARM build")
	}

	if test.Ctx().ElasticStackVersion == "7.16.0-SNAPSHOT" {
		t.Skipf("Skipping due to a known issue: https://github.com/elastic/elasticsearch/issues/80265")
	}

	srcVersion := test.LatestReleasedVersion6x
	dstVersion := test.Ctx().ElasticStackVersion

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	// covers the case where the existing zen1 master needs to be upgraded/restarted to a zen2 master
	initial := elasticsearch.NewBuilder("test-version-up-1-68x-to-7x").
		WithVersion(srcVersion).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion(dstVersion).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeTwoNodes68xTo7x(t *testing.T) {
	if test.Ctx().HasTag(test.ArchARMTag) {
		t.Skipf("Skipping test because Elasticsearch 6.8.x does not have an ARM build")
	}

	srcVersion := test.LatestReleasedVersion6x
	dstVersion := test.Ctx().ElasticStackVersion

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	// covers the case where 2 existing zen1 masters get upgraded/restarted to zen2 masters
	// due to minimum_master_nodes=2, the cluster is unavailable while the first master is upgraded
	initial := elasticsearch.NewBuilder("test-version-up-2-68x-to-7x").
		WithVersion(srcVersion).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion(dstVersion).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgrade3Nodes68xTo7x(t *testing.T) {
	if test.Ctx().HasTag(test.ArchARMTag) {
		t.Skipf("Skipping test because Elasticsearch 6.8.x does not have an ARM build")
	}

	srcVersion := test.LatestReleasedVersion6x
	dstVersion := test.Ctx().ElasticStackVersion

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	// covers the case where 3 existing zen1 masters get upgraded/restarted to zen2 masters (standard rolling upgrade)
	initial := elasticsearch.NewBuilder("test-version-up-3-68x-to-7x").
		WithVersion(srcVersion).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion(dstVersion).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeSingleMaster68xToNewNodeSet7x(t *testing.T) {
	if test.Ctx().HasTag(test.ArchARMTag) {
		t.Skipf("Skipping test because Elasticsearch 6.8.x does not have an ARM build")
	}

	if test.Ctx().ElasticStackVersion == "7.16.0-SNAPSHOT" {
		t.Skipf("Skipping due to a known issue: https://github.com/elastic/elasticsearch/issues/80265")
	}

	srcVersion := test.LatestReleasedVersion6x
	dstVersion := test.Ctx().ElasticStackVersion

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	// covers the case where the existing zen1 master get upgraded/restarted to a zen2 master
	// but the new one is specified in a different NodeSet, hence gets created before the old one is removed
	initial := elasticsearch.NewBuilder("test-version-up-68x-to-new-7x").
		WithVersion(srcVersion).
		WithNodeSet(esv1.NodeSet{
			Name:        "master68x",
			Count:       int32(1),
			PodTemplate: elasticsearch.ESPodTemplate(elasticsearch.DefaultResources),
		})

	mutated := initial.WithNoESTopology().
		WithVersion(dstVersion).
		WithNodeSet(esv1.NodeSet{
			Name:        "master7x",
			Count:       int32(1),
			PodTemplate: elasticsearch.ESPodTemplate(elasticsearch.DefaultResources),
		})

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeSingleMaster68xToMore7x(t *testing.T) {
	if test.Ctx().HasTag(test.ArchARMTag) {
		t.Skipf("Skipping test because Elasticsearch 6.8.x does not have an ARM build")
	}

	if test.Ctx().ElasticStackVersion == "7.16.0-SNAPSHOT" {
		t.Skipf("Skipping due to a known issue: https://github.com/elastic/elasticsearch/issues/80265")
	}

	srcVersion := test.LatestReleasedVersion6x
	dstVersion := test.Ctx().ElasticStackVersion

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	// covers the case where the existing zen1 master get upgraded/restarted to a zen2 master
	// but the user defines an additional zen2 master that gets created before the old one is upgraded
	initial := elasticsearch.NewBuilder("test-version-up-1-68x-more-7x").
		WithVersion(srcVersion).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion(dstVersion).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeSingleToLatest7x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestReleasedVersion7x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	initial := elasticsearch.NewBuilder("test-version-up-1-to-7x").
		WithVersion(srcVersion).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion(dstVersion).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeTwoNodesToLatest7x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestReleasedVersion7x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	initial := elasticsearch.NewBuilder("test-version-up-2-to-7x").
		WithVersion(srcVersion).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion(dstVersion).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeSingleToLatest8x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestSnapshotVersion8x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	initial := elasticsearch.NewBuilder("test-version-up-1-to-8x").
		WithVersion(srcVersion).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion(dstVersion).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeTwoNodesToLatest8x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestSnapshotVersion8x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	initial := elasticsearch.NewBuilder("test-version-up-2-to-8x").
		WithVersion(srcVersion).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion(dstVersion).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}
