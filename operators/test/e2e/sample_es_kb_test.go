// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"os"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/common"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/kibana"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Re-use the sample stack for e2e tests.
// This is a way to make sure both the sample and the e2e tests are always up-to-date.
// Path is relative to the e2e directory.
const sampleStackFile = "../../config/samples/kibana/kibana_es.yaml"

// TestStackSample runs a test suite using the sample stack
func TestStackSample(t *testing.T) {
	var es elasticsearch.Builder
	var kb kibana.Builder

	yamlFile, err := os.Open(sampleStackFile)
	helpers.ExitOnErr(err)

	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	// the decoding order depends on the yaml
	helpers.ExitOnErr(decoder.Decode(&es.Elasticsearch))
	helpers.ExitOnErr(decoder.Decode(&kb.Kibana))

	es = es.
		WithNamespace(params.Namespace).
		WithVersion(params.ElasticStackVersion).
		WithRestrictedSecurityContext()
	kb = kb.
		WithNamespace(params.Namespace).
		WithVersion(params.ElasticStackVersion).
		WithRestrictedSecurityContext()

	builders := []common.Builder{es, kb}
	// run, with mutation to the same resource (should work and do nothing)
	common.RunMutationsTests(t, builders, builders)
}
