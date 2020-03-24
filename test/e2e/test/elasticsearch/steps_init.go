// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"fmt"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/webhook"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// InitTestSteps includes pre-requisite tests (eg. is k8s accessible),
// and cleanup from previous tests
func (b Builder) InitTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "K8S should be accessible",
			Test: func(t *testing.T) {
				pods := corev1.PodList{}
				err := k.Client.List(&pods)
				require.NoError(t, err)
			},
		},
		{
			Name: "Label test pods",
			Test: func(t *testing.T) {
				err := test.LabelTestPods(
					k.Client,
					test.Ctx(),
					run.TestNameLabel,
					b.Elasticsearch.Labels[run.TestNameLabel])
				require.NoError(t, err)
			},
			Skip: func() bool {
				return test.Ctx().Local
			},
		},
		{
			Name: "Elasticsearch CRDs should exist",
			Test: func(t *testing.T) {
				crds := []runtime.Object{
					&esv1.ElasticsearchList{},
				}
				for _, crd := range crds {
					err := k.Client.List(crd)
					require.NoError(t, err)
				}
			},
		},
		{
			Name: "Webhook endpoint should not be empty",
			Test: test.Eventually(func() error {
				if test.Ctx().IgnoreWebhookFailures {
					return nil
				}
				webhookEndpoints := &corev1.Endpoints{}
				if err := k.Client.Get(types.NamespacedName{
					Namespace: test.Ctx().Operator.Namespace,
					Name:      webhook.WebhookServiceName,
				}, webhookEndpoints); err != nil {
					return err
				}
				if len(webhookEndpoints.Subsets) == 0 {
					return fmt.Errorf(
						"endpoint %s/%s is empty",
						webhookEndpoints.Namespace,
						webhookEndpoints.Name,
					)
				}
				return nil
			}),
		},
		{
			Name: "Remove Elasticsearch if it already exists",
			Test: func(t *testing.T) {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(obj)
					if err != nil {
						// might not exist, which is ok
						require.True(t, apierrors.IsNotFound(err))
					}
				}
				// wait for ES pods to disappear
				test.Eventually(func() error {
					return k.CheckPodCount(0, test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...)
				})(t)

				// it may take some extra time for Elasticsearch to be fully deleted
				test.Eventually(func() error {
					var es esv1.Elasticsearch
					err := k.Client.Get(k8s.ExtractNamespacedName(&b.Elasticsearch), &es)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					if err == nil {
						return fmt.Errorf("elasticsearch %s is still there", k8s.ExtractNamespacedName(&b.Elasticsearch))
					}
					return nil
				})(t)
			},
		},
	}
}
