// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"testing"

	v1alpha12 "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/stretchr/testify/require"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
)

// CreationTestSteps tests the creation of the given stack.
// The stack is not deleted at the end.
func CreationTestSteps(stack Builder, k *helpers.K8sHelper) helpers.TestStepList {
	return helpers.TestStepList{}.
		WithSteps(
			helpers.TestStep{
				Name: "Creating a stack should succeed",
				Test: func(t *testing.T) {
					for _, obj := range stack.RuntimeObjects() {
						err := k.Client.Create(obj)
						require.NoError(t, err)
					}
				},
			},
			helpers.TestStep{
				Name: "Stack should be created",
				Test: func(t *testing.T) {
					var createdEs v1alpha12.ElasticsearchCluster
					err := k.Client.Get(GetNamespacedName(stack), &createdEs)
					require.NoError(t, err)
					require.Equal(t, stack.Elasticsearch.Spec.Version, createdEs.Spec.Version)
					//TODO this is incomplete
				},
			},
		).
		WithSteps(CheckStackSteps(stack, k)...)
}
