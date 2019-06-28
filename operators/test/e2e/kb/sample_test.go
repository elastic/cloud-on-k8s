// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kb

import (
	"bufio"
	"os"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework/kibana"
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
	framework.ExitOnErr(err)

	// the decoding order depends on the yaml
	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	framework.ExitOnErr(decoder.Decode(&esBuilder.Elasticsearch))
	framework.ExitOnErr(decoder.Decode(&kbBuilder.Kibana))

	esBuilder = esBuilder.
		WithNamespace(framework.Namespace).
		WithVersion(framework.ElasticStackVersion).
		WithRestrictedSecurityContext()
	kbBuilder = kbBuilder.
		WithNamespace(framework.Namespace).
		WithVersion(framework.ElasticStackVersion).
		WithRestrictedSecurityContext()

	builders := []framework.Builder{esBuilder, kbBuilder}
	// run, with mutation to the same resource (should work and do nothing)
	framework.RunMutationsTests(t, builders, builders)
}
