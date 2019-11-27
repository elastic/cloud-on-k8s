// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
)

func TestVersionUpgrade680To730(t *testing.T) {
	initial := elasticsearch.NewBuilder("test-version-up-3-680-to-730").
		WithVersion("6.8.0").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion("7.3.0").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeSingle680To730(t *testing.T) {
	initial := elasticsearch.NewBuilder("test-version-up-1-680-to-730").
		WithVersion("6.8.0").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion("7.3.0").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeSingle710To730(t *testing.T) {
	initial := elasticsearch.NewBuilder("test-version-up-1-710-to-730").
		WithVersion("7.1.0").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion("7.3.0").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}

func TestVersionUpgradeSingle740To750(t *testing.T) {
	initial := elasticsearch.NewBuilder("test-version-up-1-740-to-750").
		WithVersion("7.4.2").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	mutated := initial.WithNoESTopology().
		WithVersion("7.5.0").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	RunESMutation(t, initial, mutated)
}
