// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kb

import (
	"bufio"
	"os"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/kibana"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	SampleKibanaEsStackFile = "../../../config/samples/kibana/kibana_es.yaml"
)

// TestKibanaEsSample runs a test suite using the Kibana + ES sample
func TestKibanaEsSample(t *testing.T) {
	var esBuilder elasticsearch.Builder
	var kbBuilder kibana.Builder

	yamlFile, err := os.Open(SampleKibanaEsStackFile)
	test.ExitOnErr(err)

	// the decoding order depends on the yaml
	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	test.ExitOnErr(decoder.Decode(&esBuilder.Elasticsearch))
	test.ExitOnErr(decoder.Decode(&kbBuilder.Kibana))

	esBuilder = esBuilder.
		WithNamespace(test.Namespace).
		WithVersion(test.ElasticStackVersion).
		WithRestrictedSecurityContext()
	kbBuilder = kbBuilder.
		WithNamespace(test.Namespace).
		WithVersion(test.ElasticStackVersion).
		WithRestrictedSecurityContext()

	builders := []test.Builder{esBuilder, kbBuilder}
	// run, with mutation to the same resource (should work and do nothing)
	test.RunMutations(t, builders, builders)
}
