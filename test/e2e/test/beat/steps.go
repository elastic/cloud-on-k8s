// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package beat

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/pointer"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/checks"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/generation"
)

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
					b.Beat.Labels[run.TestNameLabel])
			}),
			Skip: func() bool {
				return test.Ctx().Local
			},
		},
		{
			Name: "Beat CRDs should exist",
			Test: test.Eventually(func() error {
				crd := &beatv1beta1.BeatList{}
				return k.Client.List(context.Background(), crd)
			}),
		},
		{
			Name: "Remove Beat if it already exists",
			Test: test.Eventually(func() error {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(context.Background(), obj)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
				}
				// wait for Beat pods to disappear
				if err := k.CheckPodCount(0, test.BeatPodListOptions(b.Beat.Namespace, b.Beat.Name, b.Beat.Spec.Type)...); err != nil {
					return err
				}

				// it may take some extra time for Beat to be fully deleted
				var beat beatv1beta1.Beat
				err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Beat), &beat)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				if err == nil {
					return fmt.Errorf("beat %s is still there", k8s.ExtractNamespacedName(&b.Beat))
				}
				return nil
			}),
		},
	}
}

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}.
		WithSteps(test.StepList{
			test.Step{
				Name: "Creating a Beat should succeed",
				Test: test.Eventually(func() error {
					return k.CreateOrUpdate(b.RuntimeObjects()...)
				}),
			},
			test.Step{
				Name: "Beat should be created",
				Test: test.Eventually(func() error {
					var createdBeat beatv1beta1.Beat
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Beat), &createdBeat); err != nil {
						return err
					}
					if b.Beat.Spec.Version != createdBeat.Spec.Version {
						return fmt.Errorf("expected version %s but got %s", b.Beat.Spec.Version, createdBeat.Spec.Version)
					}
					return nil
				}),
			},
		})
}

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Beat status should be updated",
			Test: test.Eventually(func() error {
				var beat beatv1beta1.Beat
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Beat), &beat); err != nil {
					return err
				}
				// don't check association statuses that may vary across tests
				beat.Status.ElasticsearchAssociationStatus = ""
				beat.Status.KibanaAssociationStatus = ""
				beat.Status.MonitoringAssociationsStatus = nil
				beat.Status.ObservedGeneration = 0

				expected := beatv1beta1.BeatStatus{
					Version: b.Beat.Spec.Version,
					Health:  "green",
				}
				if b.Beat.Spec.Deployment != nil {
					expectedReplicas := pointer.Int32OrDefault(b.Beat.Spec.Deployment.Replicas, int32(1))
					expected.ExpectedNodes = expectedReplicas
					expected.AvailableNodes = expectedReplicas
				} else {
					// don't check the replicas count for daemonsets
					beat.Status.ExpectedNodes = 0
					beat.Status.AvailableNodes = 0
				}
				if !cmp.Equal(beat.Status, expected) {
					return fmt.Errorf("expected status %+v, got diff: %s", expected, cmp.Diff(beat.Status, expected))
				}
				return nil
			}),
		},
	}
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	// health
	return test.StepList{
		test.Step{
			Name: "Beat health should be green",
			Test: test.Eventually(func() error {
				var beat beatv1beta1.Beat
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Beat), &beat); err != nil {
					return err
				}

				if beat.Status.Health != beatv1beta1.BeatGreenHealth {
					return fmt.Errorf("beat %s health is %s", beat.Name, beat.Status.Health)
				}

				return nil
			}),
		},
		test.Step{
			Name: "ES data should pass validations",
			Test: test.Eventually(func() error {
				esNsName := b.Beat.ElasticsearchRef().WithDefaultNamespace(b.Beat.Namespace).NamespacedName()
				var es esv1.Elasticsearch
				if err := k.Client.Get(context.Background(), esNsName, &es); err != nil {
					return err
				}

				esClient, err := elasticsearch.NewElasticsearchClient(es, k)
				if err != nil {
					return err
				}

				for _, validation := range b.Validations {
					if err := validation(esClient); err != nil {
						return err
					}
				}

				return nil
			}),
		},
		checks.BeatsMonitoredStep(&b, k),
	}
}

func (b Builder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Applying the Beat mutation should succeed",
			Test: test.Eventually(func() error {
				var beat beatv1beta1.Beat
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Beat), &beat); err != nil {
					return err
				}
				beat.Spec = b.Beat.Spec
				return k.Client.Update(context.Background(), &beat)
			}),
		}}
}

func (b Builder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	return []test.Step{
		{
			Name: "Deleting the resources should return no error",
			Test: test.Eventually(func() error {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(context.Background(), obj)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
				}
				return nil
			}),
		},
		{
			Name: "The resources should not be there anymore",
			Test: test.Eventually(func() error {
				for _, obj := range b.RuntimeObjects() {
					objCopy := k8s.DeepCopyObject(obj)
					err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(obj), objCopy)
					if err != nil {
						if apierrors.IsNotFound(err) {
							continue
						}
					}
					return errors.Wrap(err, "expected 404 not found API error here")
				}
				return nil
			}),
		},
		{
			Name: "Beat pods should be eventually be removed",
			Test: test.Eventually(func() error {
				return k.CheckPodCount(0, test.BeatPodListOptions(b.Beat.Namespace, b.Beat.Name, b.Beat.Spec.Type)...)
			}),
		},
	}
}

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	var beatGenerationBeforeMutation, beatObservedGenerationBeforeMutation int64
	isMutated := b.MutatedFrom != nil

	return test.StepList{
		generation.RetrieveGenerationsStep(&b.Beat, k, &beatGenerationBeforeMutation, &beatObservedGenerationBeforeMutation),
	}.WithSteps(b.UpgradeTestSteps(k)).
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k)).
		WithStep(generation.CompareObjectGenerationsStep(&b.Beat, k, isMutated, beatGenerationBeforeMutation, beatObservedGenerationBeforeMutation))
}
