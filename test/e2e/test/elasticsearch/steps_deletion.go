// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

func (b Builder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Deleting Elasticsearch should return no error",
			Test: func(t *testing.T) {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(obj)
					require.NoError(t, err)

				}
			},
		},
		{
			Name: "Elasticsearch should not be there anymore",
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
				err := k.Client.List(&pvcs, ns, matchLabels)
				if err != nil {
					return err
				}
				if len(pvcs.Items) != 0 {
					return fmt.Errorf("%d pvcs still present", len(pvcs.Items))
				}
				return nil
			}),
		},
	}
}
