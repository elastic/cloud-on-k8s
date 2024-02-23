// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	lslabels "github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/checks"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/generation"
)

func (b Builder) InitTestSteps(k *test.K8sClient) test.StepList {
	return []test.Step{
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
					b.Logstash.Labels[run.TestNameLabel])
			}),
			Skip: func() bool {
				return test.Ctx().Local
			},
		},
		{
			Name: "Logstash CRDs should exist",
			Test: test.Eventually(func() error {
				crd := &logstashv1alpha1.LogstashList{}
				return k.Client.List(context.Background(), crd)
			}),
		},
		{
			Name: "Remove Logstash if it already exists",
			Test: test.Eventually(func() error {
				err := k.Client.Delete(context.Background(), &b.Logstash)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				// wait for pods to disappear
				return k.CheckPodCount(0, test.LogstashPodListOptions(b.Logstash.Namespace, b.Logstash.Name)...)
			}),
		},
	}
}

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Submitting the Logstash resource should succeed",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdate(&b.Logstash)
			}),
		},
		{
			Name: "Logstash should be created",
			Test: test.Eventually(func() error {
				var logstash logstashv1alpha1.Logstash
				return k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Logstash), &logstash)
			}),
		},
	}
}

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		CheckSecrets(b, k),
		CheckStatus(b, k),
		CheckServices(b, k),
		CheckServicesEndpoints(b, k),
		checks.CheckPods(b, k),
	}
}

func (b Builder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Updating the Logstash spec succeed",
			Test: test.Eventually(func() error {
				var logstash logstashv1alpha1.Logstash
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Logstash), &logstash); err != nil {
					return err
				}
				logstash.Spec = b.Logstash.Spec
				return k.Client.Update(context.Background(), &logstash)
			}),
		}}
}

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	var logstashGenerationBeforeMutation, logstashObservedGenerationBeforeMutation int64
	isMutated := b.MutatedFrom != nil
	return test.StepList{
		generation.RetrieveGenerationsStep(&b.Logstash, k, &logstashGenerationBeforeMutation, &logstashObservedGenerationBeforeMutation),
	}.WithSteps(b.UpgradeTestSteps(k)).
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k)).
		WithStep(generation.CompareObjectGenerationsStep(&b.Logstash, k, isMutated, logstashGenerationBeforeMutation, logstashObservedGenerationBeforeMutation))
}

func (b Builder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Deleting Logstash should return no error",
			Test: test.Eventually(func() error {
				err := k.Client.Delete(context.Background(), &b.Logstash)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				return nil
			}),
		},
		{
			Name: "Logstash should not be there anymore",
			Test: test.Eventually(func() error {
				objCopy := k8s.DeepCopyObject(&b.Logstash)
				err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Logstash), objCopy)
				if err != nil && apierrors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("expected 404 not found API error here. got: %w", err)
			}),
		},
		{
			Name: "Logstash pods should eventually be removed",
			Test: test.Eventually(func() error {
				return k.CheckPodCount(0, b.ListOptions()...)
			}),
		},
		{
			Name: "Cleanup any persistent volumes belonging to Logstash",
			Test: test.Eventually(func() error {
				if err := k.Client.DeleteAllOf(context.Background(), &corev1.PersistentVolumeClaim{},
					client.MatchingLabels{lslabels.NameLabelName: b.Logstash.Name},
					client.InNamespace(b.Namespace())); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				return nil
			}),
		},
	}
}
