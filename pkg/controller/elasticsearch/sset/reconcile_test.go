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

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	commonscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"

	"github.com/go-test/deep"
)

func TestReconcileStatefulSet(t *testing.T) {
	es := v1beta1.Elasticsearch{
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
	commonscheme.SetupScheme()
	err = reconciler.SetControllerReference(&es, metaObj, scheme.Scheme)
	require.NoError(t, err)

	updatedSset := *ssetSample.DeepCopy()
	updatedSset.Labels[hash.TemplateHashLabelName] = "updated"

	tests := []struct {
		name                    string
		c                       k8s.Client
		expected                v1.StatefulSet
		wantExpectationsUpdated bool
	}{
		{
			name:                    "create new sset",
			c:                       k8s.WrappedFakeClient(),
			expected:                ssetSample,
			wantExpectationsUpdated: false,
		},
		{
			name:                    "no update on existing sset",
			c:                       k8s.WrappedFakeClient(&ssetSample),
			expected:                ssetSample,
			wantExpectationsUpdated: false,
		},
		{
			name:                    "update on sset with different template hash",
			c:                       k8s.WrappedFakeClient(&ssetSample),
			expected:                updatedSset,
			wantExpectationsUpdated: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := expectations.NewExpectations(tt.c)
			returned, err := ReconcileStatefulSet(tt.c, scheme.Scheme, es, tt.expected, exp)
			require.NoError(t, err)

			// expect owner ref to be set to the es resource
			metaObj, err := meta.Accessor(&tt.expected)
			require.NoError(t, err)
			err = reconciler.SetControllerReference(&es, metaObj, scheme.Scheme)
			require.NoError(t, err)

			// check expectations were updated
			require.Equal(t, tt.wantExpectationsUpdated, len(exp.GetGenerations()) != 0)

			// returned sset should match the expected one
			diff := deep.Equal(tt.expected, returned)
			require.Nil(t, diff)
			// require.Equal(t, tt.expected, returned)
			// and be stored in the apiserver
			var retrieved appsv1.StatefulSet
			err = tt.c.Get(k8s.ExtractNamespacedName(&tt.expected), &retrieved)
			require.NoError(t, err)
			require.Equal(t, tt.expected, retrieved)
		})
	}
}
