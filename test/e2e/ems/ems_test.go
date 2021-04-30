// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package ems

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/maps"
)

// TestElasticMapsServerCrossNSAssociation tests associating Elasticsearch and Elastic Maps Server running in different namespaces.
func TestElasticMapsServerCrossNSAssociation(t *testing.T) {
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	// Elastic Maps Server is supported since 7.11.0
	if !stackVersion.GTE(version.MustParse("7.11.0")) {
		t.SkipNow()
	}

	esNamespace := test.Ctx().ManagedNamespace(0)
	emsNamespace := test.Ctx().ManagedNamespace(1)
	name := "test-cross-ns-ems-es"

	esBuilder := elasticsearch.NewBuilder(name).
		WithNamespace(esNamespace).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext()
	emsBuilder := maps.NewBuilder(name).
		WithNamespace(emsNamespace).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1).
		WithRestrictedSecurityContext()

	esWithLicense := test.LicenseTestBuilder()
	esWithLicense.BuildingThis = esBuilder

	test.Sequence(nil, test.EmptySteps, esWithLicense, emsBuilder).RunSequential(t)
}

func TestElasticMapsServerTLSDisabled(t *testing.T) {
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	// Elastic Maps Server is supported since 7.11.0
	if !stackVersion.GTE(version.MustParse("7.11.0")) {
		t.SkipNow()
	}

	name := "test-ems-tls-disabled"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext()
	emsBuilder := maps.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1).
		WithTLSDisabled(true).
		WithRestrictedSecurityContext()

	esWithLicense := test.LicenseTestBuilder()
	esWithLicense.BuildingThis = esBuilder

	test.Sequence(nil, test.EmptySteps, esWithLicense, emsBuilder).RunSequential(t)
}

func TestElasticMapsServerVersionUpgradeToLatest7x(t *testing.T) {
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	// Elastic Maps Server is supported since 7.11.0
	if !stackVersion.GTE(version.MustParse("7.11.0")) {
		t.SkipNow()
	}

	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestVersion7x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	name := "test-ems-version-upgrade"
	es := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithVersion(dstVersion)

	ems := maps.NewBuilder(name).
		WithElasticsearchRef(es.Ref()).
		WithNodeCount(2).
		WithVersion(srcVersion).
		WithRestrictedSecurityContext()

	emsUpgraded := ems.WithVersion(dstVersion).WithMutatedFrom(&ems)

	esWithLicense := test.LicenseTestBuilder()
	esWithLicense.BuildingThis = es

	test.RunMutations(t, []test.Builder{esWithLicense, ems}, []test.Builder{esWithLicense, emsUpgraded})
}
