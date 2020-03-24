// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"os"
	"testing"

	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const sampleApmEsKibanaFile = "../../config/samples/apm/apm_es_kibana.yaml"

// TestSmoke runs a test suite using the ApmServer + Kibana + ES sample.
func TestSmoke(t *testing.T) {
	var esBuilder elasticsearch.Builder
	var kbBuilder kibana.Builder
	var apmBuilder apmserver.Builder

	yamlFile, err := os.Open(sampleApmEsKibanaFile)
	test.ExitOnErr(err)

	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	// the decoding order depends on the yaml
	test.ExitOnErr(decoder.Decode(&esBuilder.Elasticsearch))
	test.ExitOnErr(decoder.Decode(&apmBuilder.ApmServer))
	test.ExitOnErr(decoder.Decode(&kbBuilder.Kibana))

	ns := test.Ctx().ManagedNamespace(0)
	randSuffix := rand.String(4)
	testName := "TestSmoke"
	esBuilder = esBuilder.
		WithSuffix(randSuffix).
		WithNamespace(ns).
		WithRestrictedSecurityContext().
		WithDefaultPersistentVolumes().
		WithLabel(run.TestNameLabel, testName).
		WithPodLabel(run.TestNameLabel, testName)
	kbBuilder = kbBuilder.
		WithSuffix(randSuffix).
		WithNamespace(ns).
		WithElasticsearchRef(esBuilder.Ref()).
		WithRestrictedSecurityContext().
		WithLabel(run.TestNameLabel, testName).
		WithPodLabel(run.TestNameLabel, testName)
	apmBuilder = apmBuilder.
		WithSuffix(randSuffix).
		WithNamespace(ns).
		WithElasticsearchRef(esBuilder.Ref()).
		WithConfig(map[string]interface{}{
			"apm-server.ilm.enabled": false,
		}).
		WithRestrictedSecurityContext().
		WithLabel(run.TestNameLabel, testName).
		WithPodLabel(run.TestNameLabel, testName)

	test.Sequence(nil, test.EmptySteps, esBuilder, kbBuilder, apmBuilder).
		RunSequential(t)
}
