// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

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

const (
	SampleApmEsKibanaFile = "../../../config/samples/apm/apm_es_kibana.yaml"
)

// TestApmEsKibanaSample runs a test suite using the sample ApmServer + ES + Kibana
func TestApmEsKibanaSample(t *testing.T) {
	var esBuilder elasticsearch.Builder
	var kbBuilder kibana.Builder
	var apmBuilder apmserver.Builder

	yamlFile, err := os.Open(SampleApmEsKibanaFile)
	test.ExitOnErr(err)

	// the decoding order depends on the yaml
	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	test.ExitOnErr(decoder.Decode(&esBuilder.Elasticsearch))
	test.ExitOnErr(decoder.Decode(&apmBuilder.ApmServer))
	test.ExitOnErr(decoder.Decode(&kbBuilder.Kibana))

	// set namespace and version
	esBuilder = esBuilder.
		WithNamespace(test.Namespace).
		WithVersion(test.ElasticStackVersion).
		WithRestrictedSecurityContext()
	kbBuilder = kbBuilder.
		WithNamespace(test.Namespace).
		WithVersion(test.ElasticStackVersion).
		WithRestrictedSecurityContext()
	apmBuilder = apmBuilder.
		WithNamespace(test.Namespace).
		WithVersion(test.ElasticStackVersion).
		WithRestrictedSecurityContext()

	test.Sequence(nil, test.EmptySteps, esBuilder, kbBuilder, apmBuilder).
		RunSequential(t)
	// TODO: is it possible to verify that it would also show up properly in Kibana?
}
