// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"os"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"

	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/apm"
	es "github.com/elastic/cloud-on-k8s/operators/test/e2e/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	kb "github.com/elastic/cloud-on-k8s/operators/test/e2e/kibana"
)

// Re-use the sample stack for e2e tests.
// This is a way to make sure both the sample and the e2e tests are always up-to-date.
// Path is relative to the e2e directory.
const sampleEsApmFile = "../../config/samples/apm/apm_es_kibana.yaml"

// TestApmEsKibanaSample runs a test suite using the sample apm server + es + kibana resources
func TestApmEsKibanaSample(t *testing.T) {
	// build resources from yaml sample
	var sampleEs es.Builder
	var sampleKb kb.Builder
	var sampleApm apm.Builder

	yamlFile, err := os.Open(sampleEsApmFile)
	helpers.ExitOnErr(err)

	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	helpers.ExitOnErr(decoder.Decode(&sampleEs.Elasticsearch))
	helpers.ExitOnErr(decoder.Decode(&sampleApm.ApmServer))
	helpers.ExitOnErr(decoder.Decode(&sampleKb.Kibana))

	// set namespace and version
	sampleEs = sampleEs.
		WithNamespace(params.Namespace).
		WithVersion(params.ElasticStackVersion).
		WithRestrictedSecurityContext()
	sampleKb = sampleKb.
		WithNamespace(params.Namespace).
		WithVersion(params.ElasticStackVersion).
		WithRestrictedSecurityContext()
	sampleApm = sampleApm.
		WithNamespace(params.Namespace).
		WithVersion(params.ElasticStackVersion).
		WithRestrictedSecurityContext()

	k := helpers.NewK8sClientOrFatal()
	helpers.TestStepList{}.
		WithSteps(es.InitTestSteps(sampleEs, k)...).
		WithSteps(kb.InitTestSteps(sampleKb, k)...).
		WithSteps(apm.InitTestSteps(sampleApm, k)...).
		WithSteps(es.CreationTestSteps(sampleEs, k)...).
		WithSteps(kb.CreationTestSteps(sampleKb, k)...).
		WithSteps(apm.CreationTestSteps(sampleApm, sampleEs.Elasticsearch, k)...).
		WithSteps(es.DeletionTestSteps(sampleEs, k)...).
		WithSteps(kb.DeletionTestSteps(sampleKb, k)...).
		WithSteps(apm.DeletionTestSteps(sampleApm, k)...).
		RunSequential(t)

	// TODO: is it possible to verify that it would also show up properly in Kibana?
}
