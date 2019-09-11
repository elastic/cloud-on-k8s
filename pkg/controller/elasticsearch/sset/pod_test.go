// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestPodName(t *testing.T) {
	require.Equal(t, "sset-2", PodName("sset", 2))
}

func TestPodNames(t *testing.T) {
	require.Equal(t,
		[]string{"sset-0", "sset-1", "sset-2"},
		PodNames(appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "sset",
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: common.Int32(3),
			},
		}),
	)
}

func Test_PodReconciliationDoneForSset(t *testing.T) {
	ssetName := "sset"
	ssetSample := func(replicas int32, partition int32, currentRev string, updateRev string) appsv1.StatefulSet {
		return TestSset{
			Name:        ssetName,
			ClusterName: "cluster",
			Replicas:    replicas,
			Partition:   partition,
			Status: appsv1.StatefulSetStatus{
				CurrentRevision: currentRev,
				UpdateRevision:  updateRev,
			},
		}.Build()
	}
	podSample := func(name string, revision string) *corev1.Pod {
		return TestPod{
			Namespace:       "ns",
			Name:            name,
			ClusterName:     "cluster",
			StatefulSetName: ssetName,
			Revision:        revision,
		}.BuildPtr()
	}

	tests := []struct {
		name        string
		c           k8s.Client
		statefulSet appsv1.StatefulSet
		want        bool
	}{
		{
			name: "statefulset with a pod missing",
			c: k8s.WrapClient(fake.NewFakeClient(
				podSample("sset-0", "current-rev"),
				podSample("sset-1", "current-rev"),
				// missing sset-2
			)),
			statefulSet: ssetSample(3, 1, "current-rev", ""),
			want:        false,
		},
		{
			name: "statefulset with an additional pod",
			c: k8s.WrapClient(fake.NewFakeClient(
				podSample("sset-0", "current-rev"),
				podSample("sset-1", "current-rev"),
				podSample("sset-2", "current-rev"),
				// sset-3 still there from previous downscale
				podSample("sset-3", "current-rev"),
			)),
			statefulSet: ssetSample(3, 1, "current-rev", ""),
			want:        false,
		},
		{
			name: "statefulset with all pods in the current revision, no upgrade revision",
			c: k8s.WrapClient(fake.NewFakeClient(
				podSample("sset-0", "current-rev"),
				podSample("sset-1", "current-rev"),
				podSample("sset-2", "current-rev"),
			)),
			statefulSet: ssetSample(3, 1, "current-rev", ""),
			want:        true,
		},
		{
			name: "statefulset with one pod (sset-2) currently being restarted (missing)",
			c: k8s.WrapClient(fake.NewFakeClient(
				podSample("sset-0", "current-rev"),
				podSample("sset-1", "current-rev"),
			)),
			statefulSet: ssetSample(3, 2, "current-rev", "update-rev"),
			want:        false,
		},
		{
			name: "statefulset with one pod upgraded, matching current partition",
			c: k8s.WrapClient(fake.NewFakeClient(
				podSample("sset-0", "current-rev"),
				podSample("sset-1", "current-rev"),
				podSample("sset-2", "update-rev"),
			)),
			statefulSet: ssetSample(3, 2, "current-rev", "update-rev"),
			want:        true,
		},
		{
			name: "statefulset with one pod not upgraded yet",
			c: k8s.WrapClient(fake.NewFakeClient(
				podSample("sset-0", "current-rev"),
				podSample("sset-1", "current-rev"),
				podSample("sset-2", "current-rev"),
			)),
			statefulSet: ssetSample(3, 2, "current-rev", "update-rev"),
			want:        false,
		},
		{
			name: "statefulset with all pods upgraded",
			c: k8s.WrapClient(fake.NewFakeClient(
				podSample("sset-0", "update-rev"),
				podSample("sset-1", "update-rev"),
				podSample("sset-2", "update-rev"),
			)),
			statefulSet: ssetSample(3, 0, "current-rev", "update-rev"),
			want:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PodReconciliationDoneForSset(tt.c, tt.statefulSet)
			require.NoError(t, err)
			if got != tt.want {
				t.Errorf("PodReconciliationDoneForSset() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test that we actually filter on the sset name and the namespace
func TestGetActualPodsForStatefulSet(t *testing.T) {
	objs := []runtime.Object{
		getPodSample("pod0", "ns0", "sset0", "clus0", "0"),
		getPodSample("pod1", "ns1", "sset0", "clus0", "0"),
		getPodSample("pod2", "ns0", "sset1", "clus1", "0"),
		getPodSample("pod3", "ns0", "sset1", "clus0", "0"),
	}
	c := k8s.WrapClient(fake.NewFakeClient(objs...))
	sset0 := getSsetSample("sset0", "ns0", "clus0")
	pods, err := GetActualPodsForStatefulSet(c, sset0)
	require.NoError(t, err)
	// only one pod is in the same stateful set and namespace
	assert.Equal(t, 1, len(pods))
}

func getSsetSample(name, namespace, clusterName string) appsv1.StatefulSet {
	return TestSset{
		Name:        name,
		Namespace:   namespace,
		ClusterName: clusterName,
		Replicas:    3,
		Partition:   1,
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "1",
			UpdateRevision:  "1",
		},
	}.Build()
}

func getPodSample(name, namespace, ssetName, clusterName, revision string) *corev1.Pod {
	return TestPod{
		Namespace:       namespace,
		Name:            name,
		ClusterName:     clusterName,
		StatefulSetName: ssetName,
		Revision:        revision,
	}.BuildPtr()
}
