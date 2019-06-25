// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"os"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/apm"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/kibana"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Re-use the sample stack for e2e tests.
// This is a way to make sure both the sample and the e2e tests are always up-to-date.
// Path is relative to the e2e directory.
const sampleEsApmFile = "../../config/samples/apm/apm_es_kibana.yaml"

// TestApmEsKibanaSample runs a test suite using the sample apm server + es + kibana resources
func TestApmEsKibanaSample(t *testing.T) {
	// build resources from yaml sample
	var es elasticsearch.Builder
	var kb kibana.Builder
	var as apm.Builder

	yamlFile, err := os.Open(sampleEsApmFile)
	helpers.ExitOnErr(err)

	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	// the decoding order depends on the yaml
	helpers.ExitOnErr(decoder.Decode(&es.Elasticsearch))
	helpers.ExitOnErr(decoder.Decode(&as.ApmServer))
	helpers.ExitOnErr(decoder.Decode(&kb.Kibana))

	// set namespace and version
	es = es.
		WithNamespace(params.Namespace).
		WithVersion(params.ElasticStackVersion).
		WithRestrictedSecurityContext()
	kb = kb.
		WithNamespace(params.Namespace).
		WithVersion(params.ElasticStackVersion).
		WithRestrictedSecurityContext()
	as = as.
		WithNamespace(params.Namespace).
		WithVersion(params.ElasticStackVersion).
		WithRestrictedSecurityContext()

	k := helpers.NewK8sClientOrFatal()

	helpers.TestStepList{}.
		WithSteps(es.InitTestSteps(k)).
		WithSteps(kb.InitTestSteps(k)).
		WithSteps(as.InitTestSteps(k)).
		WithSteps(es.CreationTestSteps(k)).
		WithSteps(kb.CreationTestSteps(k)).
		WithSteps(as.CreationTestSteps(es.Elasticsearch, k)).
		WithSteps(es.DeletionTestSteps(k)).
		WithSteps(kb.DeletionTestSteps(k)).
		WithSteps(as.DeletionTestSteps(k)).
		RunSequential(t)

	// TODO: is it possible to verify that it would also show up properly in Kibana?
}
