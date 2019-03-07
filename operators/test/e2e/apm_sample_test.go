// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"github.com/elastic/k8s-operators/operators/test/e2e/apm"
	"os"
	"testing"

	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/elastic/k8s-operators/operators/test/e2e/stack"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Re-use the sample stack for e2e tests.
// This is a way to make sure both the sample and the e2e tests are always up-to-date.
// Path is relative to the e2e directory.
const sampleEsApmFile = "../../config/samples/es_apmserver_sample.yaml"

// TestEsApmServerSample runs a test suite using the sample es + apm server resources
func TestEsApmServerSample(t *testing.T) {
	// build stack from yaml sample
	var sampleStack stack.Builder
	var sampleApm apm.Builder

	yamlFile, err := os.Open(sampleEsApmFile)
	helpers.ExitOnErr(err)

	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	helpers.ExitOnErr(decoder.Decode(&sampleApm.Association))
	helpers.ExitOnErr(decoder.Decode(&sampleStack.Elasticsearch))
	helpers.ExitOnErr(decoder.Decode(&sampleApm.ApmServer))
	helpers.ExitOnErr(decoder.Decode(&sampleStack.Association))
	helpers.ExitOnErr(decoder.Decode(&sampleStack.Kibana))

	// set namespace
	namespacedSampleStack := sampleStack.WithNamespace(helpers.DefaultNamespace)
	namespacedSampleApm := sampleApm.WithNamespace(helpers.DefaultNamespace)

	k := helpers.NewK8sClientOrFatal()
	helpers.TestStepList{}.
		WithSteps(stack.InitTestSteps(namespacedSampleStack, k)...).
		WithSteps(apm.InitTestSteps(namespacedSampleApm, k)...).
		WithSteps(stack.CreationTestSteps(namespacedSampleStack, k)...).
		WithSteps(apm.CreationTestSteps(namespacedSampleApm, k)...).
		WithSteps(stack.DeletionTestSteps(namespacedSampleStack, k)...).
		WithSteps(apm.DeletionTestSteps(namespacedSampleApm, k)...).
		RunSequential(t)

	// TODO: push apm agent events and verify they show up in Elasticsearch?
	// TODO: is it possible to verify that it would also show up properly in Kibana?
}
