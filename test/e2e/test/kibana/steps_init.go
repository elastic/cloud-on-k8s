// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"testing"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

func (b Builder) InitTestSteps(k *test.K8sClient) test.StepList {
	return []test.Step{
		{
			Name: "K8S should be accessible",
			Test: func(t *testing.T) {
				pods := corev1.PodList{}
				err := k.Client.List(&pods)
				require.NoError(t, err)
			},
		},
		{
			Name: "Label test pods",
			Test: func(t *testing.T) {
				err := test.LabelTestPods(
					k.Client,
					test.Ctx(),
					run.TestNameLabel,
					b.Kibana.Labels[run.TestNameLabel])
				require.NoError(t, err)
			},
			Skip: func() bool {
				return test.Ctx().Local
			},
		},
		{
			Name: "Kibana CRDs should exist",
			Test: func(t *testing.T) {
				crds := []runtime.Object{
					&kbv1.KibanaList{},
				}
				for _, crd := range crds {
					err := k.Client.List(crd)
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
					return k.CheckPodCount(0, test.KibanaPodListOptions(b.Kibana.Namespace, b.Kibana.Name)...)
				})(t)
			},
		},
	}
}
