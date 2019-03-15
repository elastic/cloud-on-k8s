// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"os"
	"testing"

	assoctype "github.com/elastic/k8s-operators/operators/pkg/apis/associations/v1alpha1"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/elastic/k8s-operators/operators/test/e2e/stack"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Re-use the sample stack for e2e tests.
// This is a way to make sure both the sample and the e2e tests are always up-to-date.
// Path is relative to the e2e directory.
const sampleStackFile = "../../config/samples/es_kibana_sample.yaml"

func readSampleStack() stack.Builder {
	// build stack from yaml sample
	var sampleStack stack.Builder
	yamlFile, err := os.Open(sampleStackFile)
	helpers.ExitOnErr(err)
	var es estype.Elasticsearch
	var kb kbtype.Kibana
	var assoc assoctype.KibanaElasticsearchAssociation
	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	helpers.ExitOnErr(decoder.Decode(&assoc))
	helpers.ExitOnErr(decoder.Decode(&es))
	helpers.ExitOnErr(decoder.Decode(&kb))

	sampleStack.Elasticsearch = es
	sampleStack.Kibana = kb
	sampleStack.Association = assoc

	// set namespace
	namespaced := sampleStack.WithNamespace(helpers.DefaultNamespace)

	return namespaced
}

// TestStackSample runs a test suite using the sample stack
func TestStackSample(t *testing.T) {
	s := readSampleStack()
	// run, with mutation to the same stack (should work and do nothing)
	stack.RunCreationMutationDeletionTests(t, s, s)
}

// TestStackSample_BasicLicense runs a test suite using the sample stack,
// overridden to use a Basic license
func TestStackSample_BasicLicense(t *testing.T) {
	s := readSampleStack()
	s.Elasticsearch.Spec.LicenseType = "basic"
	stack.RunCreationMutationDeletionTests(t, s, s)
}
