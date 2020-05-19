// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func (b Builder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Deleting Kibana should return no error",
			Test: func(t *testing.T) {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(obj)
					require.NoError(t, err)

				}
			},
		},
		{
			Name: "Kibana should not be there anymore",
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
			Name: "Kibana pods should eventually be removed",
			Test: test.Eventually(func() error {
				return k.CheckPodCount(0, test.KibanaPodListOptions(b.Kibana.Namespace, b.Kibana.Name)...)
			}),
		},
	}
}
