// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func (b Builder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Deleting Elasticsearch should return no error",
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
			Name: "Elasticsearch should not be there anymore",
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
			Name: "Elasticsearch pods should eventually be removed",
			Test: test.Eventually(func() error {
				return k.CheckPodCount(0, test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...)
			}),
		},
		{
			Name: "PVCs should eventually be removed",
			Test: test.Eventually(func() error {
				var pvcs corev1.PersistentVolumeClaimList
				ns := client.InNamespace(b.Elasticsearch.Namespace)
				matchLabels := client.MatchingLabels(map[string]string{
					label.ClusterNameLabelName: b.Elasticsearch.Name,
				})
				err := k.Client.List(context.Background(), &pvcs, ns, matchLabels)
				if err != nil {
					return err
				}
				if len(pvcs.Items) != 0 {
					return fmt.Errorf("%d pvcs still present", len(pvcs.Items))
				}
				return nil
			}),
			Skip: func() bool {
				return b.Elasticsearch.Spec.VolumeClaimDeletePolicy == esv1.DeleteOnScaledownOnlyPolicy
			},
		},
		{
			Name: "Soft-owned secrets should eventually be removed",
			Test: test.Eventually(func() error {
				namespace := b.Elasticsearch.Namespace
				return k.CheckSecretsRemoved([]types.NamespacedName{
					{Namespace: namespace, Name: esv1.ElasticUserSecret(b.Elasticsearch.Name)},
					{Namespace: namespace, Name: certificates.PublicCertsSecretName(esv1.ESNamer, b.Elasticsearch.Name)},
					{Namespace: namespace, Name: certificates.PublicTransportCertsSecretName(esv1.ESNamer, b.Elasticsearch.Name)},
				})
			}),
		},
	}
}
