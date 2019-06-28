// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"os"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework/apmserver"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework/kibana"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// TestSmoke runs a test suite using the ApmServer + Kibana + ES sample.
func TestSmoke(t *testing.T) {
	var esBuilder elasticsearch.Builder
	var kbBuilder kibana.Builder
	var apmBuilder apmserver.Builder

	yamlFile, err := os.Open(SampleApmEsKibanaFile)
	framework.ExitOnErr(err)

	decoder := yaml.NewYAMLToJSONDecoder(bufio.NewReader(yamlFile))
	// the decoding order depends on the yaml
	framework.ExitOnErr(decoder.Decode(&esBuilder.Elasticsearch))
	framework.ExitOnErr(decoder.Decode(&apmBuilder.ApmServer))
	framework.ExitOnErr(decoder.Decode(&kbBuilder.Kibana))

	esBuilder = esBuilder.
		WithNamespace(framework.Namespace).
		WithRestrictedSecurityContext()
	kbBuilder = kbBuilder.
		WithNamespace(framework.Namespace).
		WithRestrictedSecurityContext()
	apmBuilder = apmBuilder.
		WithNamespace(framework.Namespace).
		WithRestrictedSecurityContext()

	framework.Run(t, framework.EmptySteps, esBuilder, kbBuilder, apmBuilder)
}
