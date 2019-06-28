// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"testing"

	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
	"github.com/stretchr/testify/require"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
)

func (b Builder) CreationTestSteps(k *framework.K8sClient) framework.TestStepList {
	return framework.TestStepList{
		{
			Name: "Creating APM Server should succeed",
			Test: func(t *testing.T) {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Create(obj)
					require.NoError(t, err)
				}
			},
		},
		{
			Name: "APM Server should be created",
			Test: func(t *testing.T) {
				var createdApmServer apmtype.ApmServer
				err := k.Client.Get(k8s.ExtractNamespacedName(&b.ApmServer), &createdApmServer)
				require.NoError(t, err)
				require.Equal(t, b.ApmServer.Spec.Version, createdApmServer.Spec.Version)
				// TODO this is incomplete
			},
		},
	}
}
