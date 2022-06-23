// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package maps

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/maps"
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
					b.EMS.Labels[run.TestNameLabel])
			}),
			Skip: func() bool {
				return test.Ctx().Local
			},
		},
		{
			Name: "Elastic Map Server CRDs should exist",
			Test: test.Eventually(func() error {
				crd := &emsv1alpha1.ElasticMapsServerList{}
				return k.Client.List(context.Background(), crd)
			}),
		},
		{
			Name: "Remove Elastic Maps Server if it already exists",
			Test: test.Eventually(func() error {
				err := k.Client.Delete(context.Background(), &b.EMS)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				// wait for pods to disappear
				return k.CheckPodCount(0, test.MapsPodListOptions(b.EMS.Namespace, b.EMS.Name)...)
			}),
		},
	}
}

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Submitting the Elastic Maps Server resource should succeed",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdate(&b.EMS)
			}),
		},
		{
			Name: "Elastic Maps Server should be created",
			Test: test.Eventually(func() error {
				var ems emsv1alpha1.ElasticMapsServer
				return k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.EMS), &ems)
			}),
		},
	}
}

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		checks.CheckDeployment(b, k, maps.Deployment(b.EMS.Name)),
		checks.CheckPods(b, k),
		checks.CheckServices(b, k),
		checks.CheckServicesEndpoints(b, k),
		CheckSecrets(b, k),
		CheckStatus(b, k),
	}
}

func (b Builder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Updating the Elastic Maps Server spec succeed",
			Test: test.Eventually(func() error {
				var ems emsv1alpha1.ElasticMapsServer
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.EMS), &ems); err != nil {
					return err
				}
				ems.Spec = b.EMS.Spec
				return k.Client.Update(context.Background(), &ems)
			}),
		}}
}

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	var entSearchGenerationBeforeMutation, entSearchObservedGenerationBeforeMutation int64
	isMutated := b.MutatedFrom != nil

	return test.StepList{
		generation.RetrieveGenerationsStep(&b.EMS, k, &entSearchGenerationBeforeMutation, &entSearchObservedGenerationBeforeMutation),
	}.WithSteps(test.AnnotatePodsWithBuilderHash(b, b.MutatedFrom, k)).
		WithSteps(b.UpgradeTestSteps(k)).
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k)).
		WithStep(generation.CompareObjectGenerationsStep(&b.EMS, k, isMutated, entSearchGenerationBeforeMutation, entSearchObservedGenerationBeforeMutation))
}

func (b Builder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Deleting Elastic Maps Server should return no error",
			Test: test.Eventually(func() error {
				err := k.Client.Delete(context.Background(), &b.EMS)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				return nil
			}),
		},
		{
			Name: "Elastic Maps Server should not be there anymore",
			Test: test.Eventually(func() error {
				objCopy := k8s.DeepCopyObject(&b.EMS)
				err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.EMS), objCopy)
				if err != nil && apierrors.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("expected 404 not found API error here. got: %w", err)
			}),
		},
		{
			Name: "Elastic Maps Server pods should eventually be removed",
			Test: test.Eventually(func() error {
				return k.CheckPodCount(0, b.ListOptions()...)
			}),
		},
		{
			Name: "Soft-owned secrets should eventually be removed",
			Test: test.Eventually(func() error {
				namespace := b.EMS.Namespace
				return k.CheckSecretsRemoved([]types.NamespacedName{
					{Namespace: namespace, Name: certificates.PublicCertsSecretName(maps.EMSNamer, b.EMS.Name)},
				})
			}),
		},
	}
}
