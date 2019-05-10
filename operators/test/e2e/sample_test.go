// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"os"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/stack"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Re-use the sample stack for e2e tests.
// This is a way to make sure both the sample and the e2e tests are always up-to-date.
// Path is relative to the e2e directory.
const sampleStackFile = "../../config/samples/kibana/kibana_es.yaml"

func readSampleStack() stack.Builder {
	// build stack from yaml sample
	var sampleStack stack.Builder
	yamlFile, err := os.Open(sampleStackFile)
	helpers.ExitOnErr(err)
	var es estype.Elasticsearch
	var kb kbtype.Kibana
	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	helpers.ExitOnErr(decoder.Decode(&es))
	helpers.ExitOnErr(decoder.Decode(&kb))

	sampleStack.Elasticsearch = es
	sampleStack.Kibana = kb

	// set namespace
	namespaced := sampleStack.WithNamespace(helpers.DefaultNamespace)

	// override version to 7.0.1 until 7.1.0 is out
	// TODO remove once 7.1.0 is out
	namespaced.Elasticsearch.Spec.Version = "7.0.1"
	namespaced.Kibana.Spec.Version = "7.0.1"
	// use a trial license until 7.1.0 is out
	// TODO remove once 7.1.0 is out
	for i, n := range namespaced.Elasticsearch.Spec.Nodes {
		config := n.Config
		if config == nil {
			config = &v1alpha1.Config{}
		}
		config.Data["xpack.license.self_generated.type"] = "trial"
		namespaced.Elasticsearch.Spec.Nodes[i].Config = config
	}

	return namespaced
}

// TestStackSample runs a test suite using the sample stack
func TestStackSample(t *testing.T) {
	s := readSampleStack()
	// run, with mutation to the same stack (should work and do nothing)
	stack.RunCreationMutationDeletionTests(t, s, s)
}
