// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"context"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (b Builder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	return []test.Step{
		{
			Name: "Deleting the resources should return no error",
			Test: func(t *testing.T) {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(context.Background(), obj)
					require.NoError(t, err)

				}
			},
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
			Name: "APM Server pods should eventually be removed",
			Test: test.Eventually(func() error {
				return k.CheckPodCount(0, test.ApmServerPodListOptions(b.ApmServer.Namespace, b.ApmServer.Name)...)
			}),
		},
		{
			Name: "Soft-owned secrets should eventually be removed",
			Test: test.Eventually(func() error {
				namespace := b.ApmServer.Namespace
				return k.CheckSecretsRemoved([]types.NamespacedName{
					{Namespace: namespace, Name: apmserver.SecretToken(b.ApmServer.Name)},
					{Namespace: namespace, Name: certificates.PublicCertsSecretName(apmserver.Namer, b.ApmServer.Name)},
				})
			}),
		},
	}
}
