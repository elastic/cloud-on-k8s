// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

func TestReconcileStatefulSet(t *testing.T) {
	require.NoError(t, v1alpha1.AddToScheme(scheme.Scheme))
	es := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			UID:       types.UID("uid"),
		},
	}
	ssetSample := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      "sset",
			Labels: map[string]string{
				hash.TemplateHashLabelName: "hash-value",
			},
		},
	}
	metaObj, err := meta.Accessor(&ssetSample)
	require.NoError(t, err)
	err = controllerutil.SetControllerReference(&es, metaObj, scheme.Scheme)
	require.NoError(t, err)

	updatedSset := *ssetSample.DeepCopy()
	updatedSset.Labels[hash.TemplateHashLabelName] = "updated"

	tests := []struct {
		name     string
		c        k8s.Client
		expected v1.StatefulSet
	}{
		{
			name:     "create new sset",
			c:        k8s.WrapClient(fake.NewFakeClient()),
			expected: ssetSample,
		},
		{
			name:     "no update on existing sset",
			c:        k8s.WrapClient(fake.NewFakeClient(&ssetSample)),
			expected: ssetSample,
		},
		{
			name:     "update on sset with different template hash",
			c:        k8s.WrapClient(fake.NewFakeClient(&ssetSample)),
			expected: updatedSset,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ReconcileStatefulSet(tt.c, scheme.Scheme, es, tt.expected)
			require.NoError(t, err)

			// expect owner ref to be set to the es resource
			metaObj, err := meta.Accessor(&tt.expected)
			require.NoError(t, err)
			err = controllerutil.SetControllerReference(&es, metaObj, scheme.Scheme)
			require.NoError(t, err)

			// get back the statefulset
			var retrieved appsv1.StatefulSet
			err = tt.c.Get(k8s.ExtractNamespacedName(&tt.expected), &retrieved)
			require.NoError(t, err)
			require.Equal(t, tt.expected, retrieved)
		})
	}
}
