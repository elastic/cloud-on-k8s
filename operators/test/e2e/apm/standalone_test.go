// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

import (
	"testing"

	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/apmserver"
)

// TestApmStandalone runs a test suite using the sample ApmServer + ES + Kibana
func TestApmStandalone(t *testing.T) {

	apmBuilder := apmserver.NewBuilder("standalone").
		WithNamespace(test.Namespace).
		WithVersion(test.ElasticStackVersion).
		WithRestrictedSecurityContext().
		WithOutput(apmtype.Output{}).
		WithConfig(map[string]interface{}{
			"output.console": map[string]interface{}{
				"pretty": true,
			},
		})

	test.Sequence(nil, test.EmptySteps, apmBuilder).
		RunSequential(t)
	// TODO: is it possible to verify that it would also show up properly in Kibana?
}
