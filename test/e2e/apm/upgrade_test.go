// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build apm || e2e

package apm

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
)

func TestAPMServerVersionUpgradeToLatest8x(t *testing.T) {
	srcVersion := test.Ctx().ElasticStackVersion
	dstVersion := test.LatestReleasedVersion8x

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	name := "apmserver-upgrade"
	esBuilder := elasticsearch.NewBuilder(name).
		WithVersion(srcVersion).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	apmServerBuilder := apmserver.NewBuilder(name).WithVersion(srcVersion).WithElasticsearchRef(esBuilder.Ref()).WithoutIntegrationCheck()

	mutated := apmServerBuilder.WithVersion(dstVersion).WithElasticsearchRef(esBuilder.Ref()).WithMutatedFrom(&apmServerBuilder).WithoutIntegrationCheck()

	test.RunMutations(
		t,
		[]test.Builder{esBuilder, apmServerBuilder},
		[]test.Builder{
			esBuilder.WithVersion(dstVersion).WithMutatedFrom(&esBuilder),
			mutated,
		},
	)
}
