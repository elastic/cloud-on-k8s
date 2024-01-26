// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package sset

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	controllerscheme "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestReconcileStatefulSet(t *testing.T) {
	controllerscheme.SetupScheme()
	es := esv1.Elasticsearch{
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
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To[int32](3),
		},
	}
	metaObj, err := meta.Accessor(&ssetSample)
	require.NoError(t, err)
	err = controllerutil.SetControllerReference(&es, metaObj, scheme.Scheme)
	require.NoError(t, err)

	// simulate updated replicas & template hash label
	updatedSset := *ssetSample.DeepCopy()
	updatedSset.Spec.Replicas = ptr.To[int32](4)
	updatedSset.Labels[hash.TemplateHashLabelName] = "updated"

	tests := []struct {
		name                    string
		client                  func() k8s.Client
		expected                func() appsv1.StatefulSet
		want                    func() appsv1.StatefulSet
		wantExpectationsUpdated bool
	}{
		{
			name:                    "create new sset",
			client:                  func() k8s.Client { return k8s.NewFakeClient() },
			expected:                func() appsv1.StatefulSet { return ssetSample },
			want:                    func() appsv1.StatefulSet { return ssetSample },
			wantExpectationsUpdated: false,
		},
		{
			name:                    "no update when expected == actual",
			client:                  func() k8s.Client { return k8s.NewFakeClient(&ssetSample) },
			expected:                func() appsv1.StatefulSet { return ssetSample },
			want:                    func() appsv1.StatefulSet { return ssetSample },
			wantExpectationsUpdated: false,
		},
		{
			name:                    "update sset with different template hash",
			client:                  func() k8s.Client { return k8s.NewFakeClient(&ssetSample) },
			expected:                func() appsv1.StatefulSet { return updatedSset },
			want:                    func() appsv1.StatefulSet { return updatedSset },
			wantExpectationsUpdated: true,
		},
		{
			name: "update sset with missing template hash label",
			client: func() k8s.Client {
				ssetSampleWithMissingLabel := ssetSample.DeepCopy()
				ssetSampleWithMissingLabel.Labels = map[string]string{}
				return k8s.NewFakeClient(ssetSampleWithMissingLabel)
			},
			expected:                func() appsv1.StatefulSet { return ssetSample },
			want:                    func() appsv1.StatefulSet { return ssetSample },
			wantExpectationsUpdated: true,
		},
		{
			name: "sset update should preserve existing annotations and labels",
			client: func() k8s.Client {
				ssetSampleWithExtraMetadata := ssetSample.DeepCopy()
				// simulate annotations and labels manually set by the user
				ssetSampleWithExtraMetadata.Annotations = map[string]string{"a": "b"}
				ssetSampleWithExtraMetadata.Labels["a"] = "b"
				return k8s.NewFakeClient(ssetSampleWithExtraMetadata)
			},
			expected: func() appsv1.StatefulSet { return updatedSset },
			want: func() appsv1.StatefulSet {
				// we want the expected sset + extra metadata from the existing one
				expectedWithExtraMetadata := *updatedSset.DeepCopy()
				expectedWithExtraMetadata.Annotations = map[string]string{"a": "b"}
				expectedWithExtraMetadata.Labels["a"] = "b"
				return expectedWithExtraMetadata
			},
			wantExpectationsUpdated: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.client()
			expected := tt.expected()
			want := tt.want()
			exp := expectations.NewExpectations(client)

			returned, err := ReconcileStatefulSet(context.Background(), client, es, expected, exp)
			require.NoError(t, err)

			// returned sset should be the one we want
			comparison.AssertEqual(t, &want, &returned)
			// and be stored in the apiserver
			var retrieved appsv1.StatefulSet
			err = client.Get(context.Background(), k8s.ExtractNamespacedName(&want), &retrieved)
			require.NoError(t, err)
			comparison.AssertEqual(t, &want, &retrieved)

			// check expectations were updated
			require.Equal(t, tt.wantExpectationsUpdated, len(exp.GetGenerations()) != 0)
		})
	}
}
