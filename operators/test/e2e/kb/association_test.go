// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kb

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/kibana"
)

// TestCrossNSAssociation tests associating ElasticSearch running in a different namespace.
func TestCrossNSAssociation(t *testing.T) {
	// This test currently does not work in the E2E environment because each namespace has a dedicated
	// controller (see https://github.com/elastic/cloud-on-k8s/issues/1438)
	if !test.Ctx().Local {
		t.SkipNow()
	}

	esNamespace := test.Ctx().ManagedNamespace(0)
	kbNamespace := test.Ctx().ManagedNamespace(1)
	name := "test-cross-ns-assoc"

	esBuilder := elasticsearch.NewBuilder(name).
		WithNamespace(esNamespace).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext()

	kbBuilder := kibana.NewBuilder(name).
		WithNamespace(kbNamespace).
		WithNodeCount(1).
		WithRestrictedSecurityContext()
	kbBuilder.Kibana.Spec.ElasticsearchRef.Name = name
	kbBuilder.Kibana.Spec.ElasticsearchRef.Namespace = esNamespace

	builders := []test.Builder{esBuilder, kbBuilder}
	test.RunMutations(t, builders, builders)
}
