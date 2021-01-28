// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"fmt"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const webhookServiceName = "elastic-webhook-server"

// InitTestSteps includes pre-requisite tests (eg. is k8s accessible),
// and cleanup from previous tests
func (b Builder) InitTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "K8S should be accessible",
			Test: test.Eventually(func() error {
				pods := corev1.PodList{}
				return k.Client.List(context.Background(), &pods)
			}),
		},
		{
			Name: "Label test pods",
			Test: test.Eventually(func() error {
				return test.LabelTestPods(
					k.Client,
					test.Ctx(),
					run.TestNameLabel,
					b.Elasticsearch.Labels[run.TestNameLabel])
			}),
			Skip: func() bool {
				return test.Ctx().Local
			},
		},
		{
			Name: "Elasticsearch CRDs should exist",
			Test: test.Eventually(func() error {
				crd := &esv1.ElasticsearchList{}
				if err := k.Client.List(context.Background(), crd); err != nil {
					return err
				}
				return nil
			}),
		},
		{
			Name: "Webhook endpoint should not be empty",
			Test: test.Eventually(func() error {
				if test.Ctx().IgnoreWebhookFailures {
					return nil
				}
				webhookEndpoints := &corev1.Endpoints{}
				if err := k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: test.Ctx().Operator.Namespace,
					Name:      webhookServiceName,
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
			Test: test.Eventually(func() error {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(context.Background(), obj)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
				}
				// wait for ES pods to disappear
				if err := k.CheckPodCount(0, test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...); err != nil {
					return err
				}

				// it may take some extra time for Elasticsearch to be fully deleted
				var es esv1.Elasticsearch
				err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Elasticsearch), &es)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				if err == nil {
					return fmt.Errorf("elasticsearch %s is still there", k8s.ExtractNamespacedName(&b.Elasticsearch))
				}
				return nil
			}),
		},
	}
}
