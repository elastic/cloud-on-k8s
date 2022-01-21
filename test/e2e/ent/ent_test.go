// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build ent || e2e
// +build ent e2e

package ent

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/enterprisesearch"
)

// TestEnterpriseSearchCrossNSAssociation tests associating Elasticsearch and Enterprise Search running in different namespaces.
func TestEnterpriseSearchCrossNSAssociation(t *testing.T) {
	esNamespace := test.Ctx().ManagedNamespace(0)
	entNamespace := test.Ctx().ManagedNamespace(1)
	name := "test-cross-ns-ent-es"

	esBuilder := elasticsearch.NewBuilder(name).
		WithNamespace(esNamespace).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext()
	entBuilder := enterprisesearch.NewBuilder(name).
		WithNamespace(entNamespace).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1).
		WithRestrictedSecurityContext()

	test.Sequence(nil, test.EmptySteps, esBuilder, entBuilder).RunSequential(t)
}

func TestEnterpriseSearchTLSDisabled(t *testing.T) {
	name := "test-ent-tls-disabled"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext()
	entBuilder := enterprisesearch.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1).
		WithTLSDisabled(true).
		WithRestrictedSecurityContext()

	test.Sequence(nil, test.EmptySteps, esBuilder, entBuilder).RunSequential(t)
}

func TestEnterpriseSearchVersionUpgradeToLatest7x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestReleasedVersion7x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	name := "test-ent-version-upgrade"
	es := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithVersion(dstVersion)

	ent := enterprisesearch.NewBuilder(name).
		WithElasticsearchRef(es.Ref()).
		WithNodeCount(2).
		WithVersion(srcVersion).
		WithRestrictedSecurityContext()

	entUpgraded := ent.WithVersion(dstVersion).WithMutatedFrom(&ent)

	// During the version upgrade, the operator will toggle Enterprise Search read-only mode.
	// We don't verify this behaviour here. Instead, we just check Enterprise Search eventually
	// runs fine in the new version: it would fail to run if read-only mode wasn't toggled.
	test.RunMutations(t, []test.Builder{es, ent}, []test.Builder{es, entUpgraded})
}

func TestEnterpriseSearchVersionUpgradeToLatest8x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestSnapshotVersion8x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	name := "test-ent-version-upgrade-8x"
	es := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithVersion(srcVersion)

	ent := enterprisesearch.NewBuilder(name).
		WithElasticsearchRef(es.Ref()).
		WithNodeCount(2).
		WithVersion(srcVersion).
		WithRestrictedSecurityContext()

	esUpgraded := es.WithVersion(dstVersion).WithMutatedFrom(&es)
	entUpgraded := ent.WithVersion(dstVersion).WithMutatedFrom(&ent)

	// During the version upgrade, the operator will toggle Enterprise Search read-only mode.
	// We don't verify this behaviour here. Instead, we just check Enterprise Search eventually
	// runs fine in the new version: it would fail to run if read-only mode wasn't toggled.
	test.RunMutations(t, []test.Builder{es, ent}, []test.Builder{esUpgraded, entUpgraded})
}
