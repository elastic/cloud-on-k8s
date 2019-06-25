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

func TestSmoke(t *testing.T) {
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

	es = es.
		WithNamespace(params.Namespace).
		WithRestrictedSecurityContext()
	kb = kb.
		WithNamespace(params.Namespace).
		WithRestrictedSecurityContext()
	as = as.
		WithNamespace(params.Namespace).
		WithRestrictedSecurityContext()

	k := helpers.NewK8sClientOrFatal()
	helpers.TestStepList{}.
		WithSteps(es.InitTestSteps(k)).
		WithSteps(kb.InitTestSteps(k)).
		WithSteps(as.InitTestSteps(k)).
		WithSteps(es.CreationTestSteps(k)).
		WithSteps(kb.CreationTestSteps(k)).
		WithSteps(as.CreationTestSteps(es.Elasticsearch, k)).
		RunSequential(t)
}
