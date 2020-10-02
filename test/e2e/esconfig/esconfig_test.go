// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/esconfig"
)

func TestSLM(t *testing.T) {
	name := "test-slm"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext()
	escBuilder := esconfig.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithSLM()

	test.Sequence(nil, test.EmptySteps, esBuilder, escBuilder).RunSequential(t)
}
