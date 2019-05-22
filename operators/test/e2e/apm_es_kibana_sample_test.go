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
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/stack"
)

// Re-use the sample stack for e2e tests.
// This is a way to make sure both the sample and the e2e tests are always up-to-date.
// Path is relative to the e2e directory.
const sampleEsApmFile = "../../config/samples/apm/apm_es_kibana.yaml"

// TestApmEsKibanaSample runs a test suite using the sample apm server + es + kibana resources
func TestApmEsKibanaSample(t *testing.T) {
	// build resources from yaml sample
	var sampleStack stack.Builder
	var sampleApm apm.Builder

	yamlFile, err := os.Open(sampleEsApmFile)
	helpers.ExitOnErr(err)

	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	helpers.ExitOnErr(decoder.Decode(&sampleStack.Elasticsearch))
	helpers.ExitOnErr(decoder.Decode(&sampleApm.ApmServer))
	helpers.ExitOnErr(decoder.Decode(&sampleStack.Kibana))

	// set namespace and version
	sampleStack = sampleStack.
		WithNamespace(params.Namespace).
		WithVersion(params.ElasticStackVersion)
	sampleApm = sampleApm.
		WithNamespace(params.Namespace).
		WithVersion(params.ElasticStackVersion)

	k := helpers.NewK8sClientOrFatal()
	helpers.TestStepList{}.
		WithSteps(stack.InitTestSteps(sampleStack, k)...).
		WithSteps(apm.InitTestSteps(sampleApm, k)...).
		WithSteps(stack.CreationTestSteps(sampleStack, k)...).
		WithSteps(apm.CreationTestSteps(sampleApm, sampleStack.Elasticsearch, k)...).
		WithSteps(stack.CheckStackSteps(sampleStack, k)...).
		WithSteps(apm.CheckStackSteps(sampleApm, sampleStack.Elasticsearch, k)...).
		WithSteps(stack.DeletionTestSteps(sampleStack, k)...).
		WithSteps(apm.DeletionTestSteps(sampleApm, k)...).
		RunSequential(t)

	// TODO: is it possible to verify that it would also show up properly in Kibana?
}
