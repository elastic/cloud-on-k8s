// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"os"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/kibana"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// TestSmoke runs a test suite using the ApmServer + Kibana + ES sample.
func TestSmoke(t *testing.T) {
	var esBuilder elasticsearch.Builder
	var kbBuilder kibana.Builder
	var apmBuilder apmserver.Builder

	yamlFile, err := os.Open(SampleApmEsKibanaFile)
	test.ExitOnErr(err)

	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	// the decoding order depends on the yaml
	test.ExitOnErr(decoder.Decode(&esBuilder.Elasticsearch))
	test.ExitOnErr(decoder.Decode(&apmBuilder.ApmServer))
	test.ExitOnErr(decoder.Decode(&kbBuilder.Kibana))

	ns := test.Ctx().ManagedNamespace(0)
	esBuilder = esBuilder.
		WithRandomPrefixName().
		WithNamespace(ns).
		WithRestrictedSecurityContext()
	kbBuilder = kbBuilder.
		WithRandomPrefixName().
		WithNamespace(ns).
		WithElasticsearchRef(esBuilder.Ref()).
		WithRestrictedSecurityContext()
	apmBuilder = apmBuilder.
		WithRandomPrefixName().
		WithNamespace(ns).
		WithElasticsearchRef(esBuilder.Ref()).
		WithRestrictedSecurityContext()

	test.Sequence(nil, test.EmptySteps, esBuilder, kbBuilder, apmBuilder).
		RunSequential(t)
}
