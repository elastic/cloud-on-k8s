// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestScheduledUpgradesDone(t *testing.T) {
	ssetSample := func(name string, partition int32, currentRev string, updateRev string) appsv1.StatefulSet {
		return appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      name,
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: common.Int32(3),
				UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
					Type:          appsv1.RollingUpdateStatefulSetStrategyType,
					RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{Partition: common.Int32(partition)},
				},
			},
			Status: appsv1.StatefulSetStatus{
				CurrentRevision: currentRev,
				UpdateRevision:  updateRev,
			},
		}
	}
	podSample := func(name string, revision string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      name,
				Labels: map[string]string{
					appsv1.StatefulSetRevisionLabel: revision,
				},
			},
		}
	}

	tests := []struct {
		name         string
		c            k8s.Client
		statefulSets StatefulSetList
		want         bool
	}{
		{
			name:         "no statefulset",
			c:            k8s.WrapClient(fake.NewFakeClient()),
			statefulSets: nil,
			want:         true,
		},
		{
			name:         "statefulset with no upgrade revision",
			c:            k8s.WrapClient(fake.NewFakeClient()),
			statefulSets: StatefulSetList{ssetSample("sset", 1, "current-rev", "")},
			want:         true,
		},
		{
			name: "statefulset with one pod (sset-2) currently being restarted (missing)",
			c: k8s.WrapClient(fake.NewFakeClient(
				podSample("sset-0", "current-rev"),
				podSample("sset-1", "current-rev"),
			)),
			statefulSets: StatefulSetList{ssetSample("sset", 2, "current-rev", "update-rev")},
			want:         false,
		},
		{
			name: "statefulset with one pod upgraded, matching current partition",
			c: k8s.WrapClient(fake.NewFakeClient(
				podSample("sset-0", "current-rev"),
				podSample("sset-1", "current-rev"),
				podSample("sset-2", "update-rev"),
			)),
			statefulSets: StatefulSetList{ssetSample("sset", 2, "current-rev", "update-rev")},
			want:         true,
		},
		{
			name: "statefulset with one pod not upgraded yet",
			c: k8s.WrapClient(fake.NewFakeClient(
				podSample("sset-0", "current-rev"),
				podSample("sset-1", "current-rev"),
				podSample("sset-2", "current-rev"),
			)),
			statefulSets: StatefulSetList{ssetSample("sset", 2, "current-rev", "update-rev")},
			want:         false,
		},
		{
			name: "statefulset with all pods upgraded",
			c: k8s.WrapClient(fake.NewFakeClient(
				podSample("sset-0", "update-rev"),
				podSample("sset-1", "update-rev"),
				podSample("sset-2", "update-rev"),
			)),
			statefulSets: StatefulSetList{ssetSample("sset", 0, "current-rev", "update-rev")},
			want:         true,
		},
		{
			name: "multiple statefulsets with all pods upgraded",
			c: k8s.WrapClient(fake.NewFakeClient(
				podSample("sset-0", "update-rev"),
				podSample("sset-1", "update-rev"),
				podSample("sset-2", "update-rev"),
				podSample("sset2-0", "update-rev"),
				podSample("sset2-1", "update-rev"),
				podSample("sset2-2", "update-rev"),
			)),
			statefulSets: StatefulSetList{
				ssetSample("sset", 0, "current-rev", "update-rev"),
				ssetSample("sset2", 0, "current-rev", "update-rev"),
			},
			want: true,
		},
		{
			name: "multiple statefulsets with some pods not upgraded yet",
			c: k8s.WrapClient(fake.NewFakeClient(
				podSample("sset-0", "update-rev"),
				podSample("sset-1", "update-rev"),
				podSample("sset-2", "update-rev"),
				podSample("sset2-0", "update-rev"),
				podSample("sset2-1", "update-rev"),
				podSample("sset2-2", "current-rev"), // not upgraded yet
				podSample("sset3-0", "update-rev"),
				podSample("sset3-1", "update-rev"),
				podSample("sset3-2", "update-rev"),
			)),
			statefulSets: StatefulSetList{
				ssetSample("sset", 0, "current-rev", "update-rev"),
				ssetSample("sset2", 0, "current-rev", "update-rev"),
				ssetSample("sset3", 0, "current-rev", "update-rev"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ScheduledUpgradesDone(tt.c, tt.statefulSets)
			require.NoError(t, err)
			if got != tt.want {
				t.Errorf("ScheduledUpgradesDone() got = %v, want %v", got, tt.want)
			}
		})
	}
}
