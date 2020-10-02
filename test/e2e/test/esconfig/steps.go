// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	"fmt"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	escv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/esconfig/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func (b Builder) InitTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "K8S should be accessible",
			Test: test.Eventually(func() error {
				pods := corev1.PodList{}
				return k.Client.List(&pods)
			}),
		},
		{
			Name: "Label test pods",
			Test: test.Eventually(func() error {
				return test.LabelTestPods(
					k.Client,
					test.Ctx(),
					run.TestNameLabel,
					b.ElasticsearchConfig.Labels[run.TestNameLabel])
			}),
			Skip: func() bool {
				return test.Ctx().Local
			},
		},
		{
			Name: "ES Config CRD should exist",
			Test: test.Eventually(func() error {
				crds := []runtime.Object{
					&escv1alpha1.ElasticsearchConfigList{},
				}
				for _, crd := range crds {
					if err := k.Client.List(crd); err != nil {
						return err
					}
				}
				return nil
			}),
		},
		{
			Name: "Remove ES Config if it already exists",
			Test: test.Eventually(func() error {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(obj)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
				}

				// it may take some extra time for Beat to be fully deleted
				var esc escv1alpha1.ElasticsearchConfig
				err := k.Client.Get(k8s.ExtractNamespacedName(&b.ElasticsearchConfig), &esc)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				if err == nil {
					return errors.New(fmt.Sprintf("esconfig %s is still there", k8s.ExtractNamespacedName(&b.ElasticsearchConfig)))
				}
				return nil
			}),
		},
	}
}

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Creating ElasticsearchConfig should succeed",
			Test: func(t *testing.T) {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Create(obj)
					require.NoError(t, err)
				}
			},
		},
		{
			Name: "ElasticsearchConfig should be created",
			Test: func(t *testing.T) {
				var createdEsc escv1alpha1.ElasticsearchConfig
				err := k.Client.Get(k8s.ExtractNamespacedName(&b.ElasticsearchConfig), &createdEsc)
				require.NoError(t, err)
			},
		},
	}
}

// TODO Check ES config status when we start updating it
func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		test.Step{
			Name: "Check Elasticsearch settings",
			Test: test.Eventually(func() error {
				esNsn := types.NamespacedName{
					Namespace: b.ElasticsearchConfig.Spec.ElasticsearchRef.Namespace,
					Name:      b.ElasticsearchConfig.Spec.ElasticsearchRef.Name,
				}
				var es esv1.Elasticsearch
				if err := k.Client.Get(esNsn, &es); err != nil {
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
	}
}

func (b Builder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Applying the ElasticsearchConfig mutation should succeed",
			Test: func(t *testing.T) {
				var esc escv1alpha1.ElasticsearchConfig
				require.NoError(t, k.Client.Get(k8s.ExtractNamespacedName(&b.ElasticsearchConfig), &esc))
				esc.Spec = b.ElasticsearchConfig.Spec
				require.NoError(t, k.Client.Update(&esc))
			},
		}}
}

// TODO should this also remove ES?
func (b Builder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	return []test.Step{
		{
			Name: "Deleting the resources should return no error",
			Test: func(t *testing.T) {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(obj)
					require.NoError(t, err)
				}
			},
		},
		{
			Name: "The resources should not be there anymore",
			Test: test.Eventually(func() error {
				for _, obj := range b.RuntimeObjects() {
					m, err := meta.Accessor(obj)
					if err != nil {
						return err
					}
					err = k.Client.Get(k8s.ExtractNamespacedName(m), obj.DeepCopyObject())
					if err != nil {
						if apierrors.IsNotFound(err) {
							continue
						}
					}
					return errors.Wrap(err, "expected 404 not found")
				}
				return nil
			}),
		},
	}
}

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	return b.UpgradeTestSteps(k).
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k))
}

func (b Builder) MutationReversalTestContext() test.ReversalTestContext {
	panic("unimplemented")
}
