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

func (b Builder) CreationTestSteps(es estype.Elasticsearch, k *helpers.K8sHelper) helpers.TestStepList {
	return helpers.TestStepList{}.
		WithSteps(helpers.TestStepList{
			helpers.TestStep{
				Name: "Creating apmserver should succeed",
				Test: func(t *testing.T) {
					for _, obj := range b.RuntimeObjects() {
						err := k.Client.Create(obj)
						require.NoError(t, err)
					}
				},
			},
			helpers.TestStep{
				Name: "apmserver should be created",
				Test: func(t *testing.T) {
					var createdApmServer apmtype.ApmServer
					err := k.Client.Get(k8s.ExtractNamespacedName(&b.ApmServer), &createdApmServer)
					require.NoError(t, err)
					require.Equal(t, b.ApmServer.Spec.Version, createdApmServer.Spec.Version)
					//TODO this is incomplete
				},
			},
		}).
		WithSteps(b.CheckStackSteps(es, k))
}
