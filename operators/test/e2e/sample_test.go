// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"os"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/elastic/k8s-operators/operators/test/e2e/stack"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Re-use the sample stack for e2e tests.
// This is a way to make sure both the sample and the e2e tests are always up-to-date.
// Path is relative to the e2e directory.
const sampleStackFile = "../../config/samples/deployments_v1alpha1_stack.yaml"

// TestStackSample runs a test suite using the sample stack
func TestStackSample(t *testing.T) {
	// build stack from yaml sample
	var sampleStack v1alpha1.Stack
	yamlFile, err := os.Open(sampleStackFile)
	helpers.ExitOnErr(err)
	err = yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile)).Decode(&sampleStack)
	helpers.ExitOnErr(err)

	// set namespace
	sampleStack.ObjectMeta.Namespace = helpers.DefaultNamespace

	// run, with mutation to the same stack (should work and do nothing)
	stack.RunCreationMutationDeletionTests(t, sampleStack, sampleStack)
}
