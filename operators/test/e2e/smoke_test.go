// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"os"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/apm"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/stack"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestSmoke(t *testing.T) {
	var sampleStack stack.Builder
	var sampleApm apm.Builder

	yamlFile, err := os.Open(sampleEsApmFile)
	helpers.ExitOnErr(err)

	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	helpers.ExitOnErr(decoder.Decode(&sampleStack.Elasticsearch))
	helpers.ExitOnErr(decoder.Decode(&sampleApm.ApmServer))
	helpers.ExitOnErr(decoder.Decode(&sampleStack.Kibana))

	namespacedSampleStack := sampleStack.WithNamespace(helpers.DefaultNamespace)
	namespacedSampleApm := sampleApm.WithNamespace(helpers.DefaultNamespace)

	k := helpers.NewK8sClientOrFatal()
	helpers.TestStepList{}.
		WithSteps(stack.InitTestSteps(namespacedSampleStack, k)...).
		WithSteps(apm.InitTestSteps(namespacedSampleApm, k)...).
		WithSteps(stack.CreationTestSteps(namespacedSampleStack, k)...).
		WithSteps(apm.CreationTestSteps(namespacedSampleApm, namespacedSampleStack.Elasticsearch, k)...).
		RunSequential(t)
}
