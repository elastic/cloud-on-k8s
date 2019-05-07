// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

import (
	"testing"

	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InitTestSteps includes pre-requisite tests (eg. is k8s accessible),
// and cleanup from previous tests
func InitTestSteps(stack Builder, k *helpers.K8sHelper) []helpers.TestStep {
	return []helpers.TestStep{

		{
			Name: "K8S should be accessible",
			Test: func(t *testing.T) {
				pods := corev1.PodList{}
				err := k.Client.List(&client.ListOptions{}, &pods)
				require.NoError(t, err)
			},
		},

		{
			Name: "ApmServer CRDs should exist",
			Test: func(t *testing.T) {
				err := k.Client.List(&client.ListOptions{}, &apmtype.ApmServerList{})
				require.NoError(t, err)
			},
		},

		{
			Name: "Remove the resources if they already exist",
			Test: func(t *testing.T) {
				for _, obj := range stack.RuntimeObjects() {
					err := k.Client.Delete(obj)
					if err != nil {
						// might not exist, which is ok
						require.True(t, apierrors.IsNotFound(err))
					}
				}
				// wait for ES pods to disappear
				helpers.Eventually(func() error {
					return k.CheckPodCount(helpers.ApmServerPodListOptions(stack.ApmServer.Name), 0)
				})(t)
			},
		},
	}
}
