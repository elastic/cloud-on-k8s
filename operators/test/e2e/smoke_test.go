// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"os"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/apm"
	es "github.com/elastic/cloud-on-k8s/operators/test/e2e/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	kb "github.com/elastic/cloud-on-k8s/operators/test/e2e/kibana"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestSmoke(t *testing.T) {
	var sampleEs es.Builder
	var sampleKb kb.Builder
	var sampleApm apm.Builder

	yamlFile, err := os.Open(sampleEsApmFile)
	helpers.ExitOnErr(err)

	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	helpers.ExitOnErr(decoder.Decode(&sampleEs.Elasticsearch))
	helpers.ExitOnErr(decoder.Decode(&sampleKb.Kibana))
	helpers.ExitOnErr(decoder.Decode(&sampleApm.ApmServer))

	sampleEs = sampleEs.
		WithNamespace(params.Namespace).
		WithRestrictedSecurityContext()
	sampleKb = sampleKb.
		WithNamespace(params.Namespace).
		WithRestrictedSecurityContext()
	sampleApm = sampleApm.
		WithNamespace(params.Namespace).
		WithRestrictedSecurityContext()

	k := helpers.NewK8sClientOrFatal()
	helpers.TestStepList{}.
		WithSteps(es.InitTestSteps(sampleEs, k)...).
		WithSteps(kb.InitTestSteps(sampleKb, k)...).
		WithSteps(apm.InitTestSteps(sampleApm, k)...).
		WithSteps(es.CreationTestSteps(sampleEs, k)...).
		WithSteps(kb.CreationTestSteps(sampleKb, k)...).
		WithSteps(apm.CreationTestSteps(sampleApm, sampleEs.Elasticsearch, k)...).
		RunSequential(t)
}
