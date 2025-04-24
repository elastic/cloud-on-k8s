// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

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
	srcVersion, dstVersion := test.GetUpgradePathTo8x(test.Ctx().ElasticStackVersion)

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	// set CPU requests and memory limits, so the desired nodes API is used during an upgrade
	resources := corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU: resource.MustParse("1"),
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}

	initial := elasticsearch.NewBuilder("test-version-up-1-to-8x").
		WithVersion(srcVersion).
		WithESMasterDataNodes(1, resources)

	mutated := initial.WithNoESTopology().
		WithVersion(dstVersion).
		WithESMasterDataNodes(1, resources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeTwoNodesToLatest8x(t *testing.T) {
	srcVersion, dstVersion := test.GetUpgradePathTo8x(test.Ctx().ElasticStackVersion)

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	initial := elasticsearch.NewBuilder("test-version-up-2-to-8x").
		WithVersion(srcVersion).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion(dstVersion).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}
