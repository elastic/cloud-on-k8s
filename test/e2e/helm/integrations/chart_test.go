// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build chart_integrations || e2e

package integrations

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func TestDefaultKubernetes(t *testing.T) {

	namespace := test.Ctx().ManagedNamespace(0)
	stackVersion := test.Ctx().ElasticStackVersion
	suffix := rand.String(4)

	fullTestName := fmt.Sprintf("test-integrations-helm-%s", suffix)

	esBuilder, err := getElasticSearchBuilder(namespace, suffix, fullTestName, stackVersion)
	require.NoError(t, err)

	chBuilder, err := newChartBuilder(fullTestName, namespace, esBuilder.Elasticsearch, map[string]interface{}{
		"kubernetes": map[string]interface{}{
			"enabled": true,
		},
		"cloudDefend": map[string]interface{}{
			"enabled": false,
		},
		"eck_agent": map[string]interface{}{
			"version": stackVersion,
		},
	})
	require.NoError(t, err)

	test.BeforeAfterSequence(nil, nil, esBuilder, chBuilder).RunSequential(t)

}
