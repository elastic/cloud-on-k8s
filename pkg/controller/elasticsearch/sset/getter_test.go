// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
)

func TestGetClaim(t *testing.T) {
	tests := []struct {
		name      string
		claims    []corev1.PersistentVolumeClaim
		claimName string
		want      *corev1.PersistentVolumeClaim
	}{
		{
			name: "return matching claim",
			claims: []corev1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "claim1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "claim2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "claim3"}},
			},
			claimName: "claim2",
			want:      &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "claim2"}},
		},
		{
			name: "return nil if no match",
			claims: []corev1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "claim1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "claim2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "claim3"}},
			},
			claimName: "claim4",
			want:      nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetClaim(tt.claims, tt.claimName); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetClaim() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetrieveActualPVCs(t *testing.T) {
	// 3 replicas, 2 PVCs each
	sset := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: pointer.Int32Ptr(3),
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "claim1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "claim2"}},
			},
		},
	}
	pvcs := []corev1.PersistentVolumeClaim{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim1-sset-0"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim2-sset-0"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim1-sset-1"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim2-sset-1"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim1-sset-2"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim2-sset-2"}},
	}
	expected := map[string][]corev1.PersistentVolumeClaim{
		"claim1": {
			{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim1-sset-0"}},
			{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim1-sset-1"}},
			{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim1-sset-2"}},
		},
		"claim2": {
			{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim2-sset-0"}},
			{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim2-sset-1"}},
			{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim2-sset-2"}},
		},
	}
	asRuntimeObjs := func(pvcs []corev1.PersistentVolumeClaim) []runtime.Object {
		objs := make([]runtime.Object, 0, len(pvcs))
		for i := range pvcs {
			objs = append(objs, &pvcs[i])
		}
		return objs
	}

	tests := []struct {
		name        string
		k8sClient   k8s.Client
		statefulSet appsv1.StatefulSet
		want        map[string][]corev1.PersistentVolumeClaim
	}{
		{
			name:        "return expected PVCs for the StatefulSet",
			k8sClient:   k8s.WrappedFakeClient(asRuntimeObjs(pvcs)...),
			statefulSet: sset,
			want:        expected,
		},
		{
			name:        "some PVCs are missing: return what can be returned",
			k8sClient:   k8s.WrappedFakeClient(&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim1-sset-0"}}),
			statefulSet: sset,
			want:        map[string][]corev1.PersistentVolumeClaim{"claim1": {{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim1-sset-0"}}}},
		},
		{
			name:        "extra PVCs exist but are not expected: don't return them",
			k8sClient:   k8s.WrappedFakeClient(asRuntimeObjs(append(pvcs, corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "claim1-sset-3"}}))...),
			statefulSet: sset,
			want:        expected,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RetrieveActualPVCs(tt.k8sClient, tt.statefulSet)
			require.NoError(t, err)
			require.Equal(t, len(got), len(tt.want))
			for claim, pvcs := range tt.want {
				gotPVCs, exists := got[claim]
				require.True(t, exists)
				require.Equal(t, len(pvcs), len(gotPVCs))
				for i := range pvcs {
					comparison.RequireEqual(t, &gotPVCs[i], &pvcs[i])
				}
			}
		})
	}
}
