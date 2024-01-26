// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	controllerscheme "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_recreateStatefulSets(t *testing.T) {
	controllerscheme.SetupScheme()
	es := func() *esv1.Elasticsearch {
		return &esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es", UID: "es-uid"}, TypeMeta: metav1.TypeMeta{Kind: esv1.Kind}}
	}
	withAnnotation := func(es *esv1.Elasticsearch, key, value string) *esv1.Elasticsearch {
		if es.Annotations == nil {
			es.Annotations = map[string]string{}
		}
		es.Annotations[key] = value
		return es
	}

	sset1 := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1", UID: "sset1-uid"}}
	sset1Bytes, _ := json.Marshal(sset1)
	sset1JSON := string(sset1Bytes)
	sset1DifferentUID := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1", UID: "sset1-differentuid"}}
	pod1 := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1-0", Labels: map[string]string{
		label.StatefulSetNameLabelName: sset1.Name,
	}}}
	pod1WithOwnerRef := pod1.DeepCopy()
	require.NoError(t, controllerutil.SetOwnerReference(es(), pod1WithOwnerRef, scheme.Scheme))

	sset2 := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset2", UID: "sset2-uid"}}
	sset2Bytes, _ := json.Marshal(sset2)
	sset2JSON := string(sset2Bytes)

	type args struct {
		runtimeObjs []client.Object
		es          esv1.Elasticsearch
	}
	tests := []struct {
		name string
		args
		wantES          esv1.Elasticsearch
		wantSsets       []appsv1.StatefulSet
		wantPods        []corev1.Pod
		wantRecreations int
	}{
		{
			name: "no annotation: nothing to do",
			args: args{
				runtimeObjs: []client.Object{sset1, pod1},
				es:          *es(),
			},
			wantES:          *es(),
			wantPods:        []corev1.Pod{*pod1},
			wantRecreations: 0,
		},
		{
			name: "StatefulSet to delete",
			args: args{
				runtimeObjs: []client.Object{sset1, pod1}, // sset exists with the same UID
				es:          *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
			},
			wantES:          *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
			wantSsets:       nil,                             // deleted
			wantPods:        []corev1.Pod{*pod1WithOwnerRef}, // owner ref set to the ES resource
			wantRecreations: 1,
		},
		{
			name: "StatefulSet to create",
			args: args{
				runtimeObjs: []client.Object{pod1}, // sset doesn't exist
				es:          *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
			},
			wantES: *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
			// created, no UUID due to how the fake client creates objects
			wantSsets:       []appsv1.StatefulSet{{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1"}}},
			wantPods:        []corev1.Pod{*pod1}, // unmodified
			wantRecreations: 1,
		},
		{
			name: "StatefulSet already recreated: remove the annotation",
			args: args{
				runtimeObjs: []client.Object{sset1DifferentUID, pod1WithOwnerRef}, // sset recreated
				es:          *withAnnotation(es(), "elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
			},
			wantES:          *es(),                                    // annotation removed
			wantSsets:       []appsv1.StatefulSet{*sset1DifferentUID}, // same
			wantPods:        []corev1.Pod{*pod1},                      // ownerRef removed
			wantRecreations: 0,
		},
		{
			name: "multiple statefulsets to handle",
			args: args{
				runtimeObjs: []client.Object{sset1, sset2, pod1},
				es: *withAnnotation(withAnnotation(es(),
					"elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
					"elasticsearch.k8s.elastic.co/recreate-sset2", sset2JSON),
			},
			wantES: *withAnnotation(withAnnotation(es(),
				"elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
				"elasticsearch.k8s.elastic.co/recreate-sset2", sset2JSON),
			wantSsets:       nil,
			wantPods:        []corev1.Pod{*pod1WithOwnerRef}, // ownerRef removed
			wantRecreations: 2,
		},
		{
			name: "additional annotations are ignored",
			args: args{
				runtimeObjs: []client.Object{sset1DifferentUID, pod1}, // sset recreated
				es: *withAnnotation(withAnnotation(es(),
					"elasticsearch.k8s.elastic.co/recreate-sset1", sset1JSON),
					"another-annotation-key", sset2JSON),
			},
			// sset annotation removed, other annotation preserved
			wantES:          *withAnnotation(es(), "another-annotation-key", sset2JSON),
			wantSsets:       nil,
			wantPods:        []corev1.Pod{*pod1},
			wantRecreations: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient(append(tt.args.runtimeObjs, &tt.args.es)...)
			got, err := recreateStatefulSets(context.Background(), k8sClient, tt.args.es)
			require.NoError(t, err)
			require.Equal(t, tt.wantRecreations, got)

			var retrievedES esv1.Elasticsearch
			err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&tt.args.es), &retrievedES)
			require.NoError(t, err)
			comparison.RequireEqual(t, &tt.wantES, &retrievedES)

			var retrievedSsets appsv1.StatefulSetList
			err = k8sClient.List(context.Background(), &retrievedSsets)
			require.NoError(t, err)
			for i := range tt.wantSsets {
				comparison.RequireEqual(t, &tt.wantSsets[i], &retrievedSsets.Items[i])
			}

			var retrievedPods corev1.PodList
			err = k8sClient.List(context.Background(), &retrievedPods)
			require.NoError(t, err)
			for i := range tt.wantPods {
				comparison.RequireEqual(t, &tt.wantPods[i], &retrievedPods.Items[i])
			}
		})
	}
}
