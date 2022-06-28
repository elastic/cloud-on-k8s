// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package enterprisesearch

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func (b Builder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Deleting EnterpriseSearch should return no error",
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
			Name: "EnterpriseSearch should not be there anymore",
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
			Name: "EnterpriseSearch pods should eventually be removed",
			Test: test.Eventually(func() error {
				return k.CheckPodCount(0, test.EnterpriseSearchPodListOptions(b.EnterpriseSearch.Namespace, b.EnterpriseSearch.Name)...)
			}),
		},
		{
			Name: "Soft-owned secrets should eventually be removed",
			Test: test.Eventually(func() error {
				namespace := b.EnterpriseSearch.Namespace
				return k.CheckSecretsRemoved([]types.NamespacedName{
					{Namespace: namespace, Name: certificates.PublicCertsSecretName(entv1.Namer, b.EnterpriseSearch.Name)},
				})
			}),
		},
	}
}
