// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

import (
	"testing"

	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/stretchr/testify/require"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
)

// CreationTestSteps tests the creation of the given stack.
// The stack is not deleted at the end.
func CreationTestSteps(stack Builder, es estype.Elasticsearch, k *helpers.K8sHelper) helpers.TestStepList {
	return helpers.TestStepList{}.
		WithSteps(
			helpers.TestStep{
				Name: "Creating resources should succeed",
				Test: func(t *testing.T) {
					for _, obj := range stack.RuntimeObjects() {
						err := k.Client.Create(obj)
						require.NoError(t, err)
					}
				},
			},
			helpers.TestStep{
				Name: "Resources should be created",
				Test: func(t *testing.T) {
					var createdApmServer apmtype.ApmServer
					err := k.Client.Get(k8s.ExtractNamespacedName(&stack.ApmServer), &createdApmServer)
					require.NoError(t, err)
					require.Equal(t, stack.ApmServer.Spec.Version, createdApmServer.Spec.Version)
					//TODO this is incomplete
				},
			},
		).
		WithSteps(CheckStackSteps(stack, es, k)...)
}
