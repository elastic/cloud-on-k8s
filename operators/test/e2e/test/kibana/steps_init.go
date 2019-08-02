// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"testing"

	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (b Builder) InitTestSteps(k *test.K8sClient) test.StepList {
	return []test.Step{
		{
			Name: "K8S should be accessible",
			Test: func(t *testing.T) {
				pods := corev1.PodList{}
				err := k.Client.List(&client.ListOptions{}, &pods)
				require.NoError(t, err)
			},
		},
		{
			Name: "Kibana CRDs should exist",
			Test: func(t *testing.T) {
				crds := []runtime.Object{
					&kbtype.KibanaList{},
				}
				for _, crd := range crds {
					err := k.Client.List(&client.ListOptions{}, crd)
					require.NoError(t, err)
				}
			},
		},
		{
			Name: "Remove Kibana if it already exists",
			Test: func(t *testing.T) {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(obj)
					if err != nil {
						// might not exist, which is ok
						require.True(t, apierrors.IsNotFound(err))
					}
				}
				// wait for Kibana pods to disappear
				test.Eventually(func() error {
					return k.CheckPodCount(test.KibanaPodListOptions(b.Kibana.Namespace, b.Kibana.Name), 0)
				})(t)
			},
		},
	}
}
