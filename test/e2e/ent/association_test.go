// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

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
