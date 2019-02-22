// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// DeletionTestSteps tests the deletion of the given stack
func DeletionTestSteps(stack Builder, k *helpers.K8sHelper) []helpers.TestStep {
	return []helpers.TestStep{
		{
			Name: "Deleting stack should return no error",
			Test: func(t *testing.T) {
				for _, obj := range stack.RuntimeObjects() {
					err := k.Client.Delete(obj)
					require.NoError(t, err)

				}
			},
		},
		{
			Name: "Stack should not be there anymore",
			Test: helpers.Eventually(func() error {
				for _, obj := range stack.RuntimeObjects() {
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
					return errors.New("Expected 404 not found API error here")

				}
				return nil
			}),
		},
		{
			Name: "ES pods should be eventually be removed",
			Test: helpers.Eventually(func() error {
				return k.CheckPodCount(helpers.ESPodListOptions(stack.Elasticsearch.Name), 0)
			}),
		},
	}
}
