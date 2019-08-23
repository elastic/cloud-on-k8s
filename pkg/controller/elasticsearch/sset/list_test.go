// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var ssetv7 = appsv1.StatefulSet{
	Spec: appsv1.StatefulSetSpec{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					label.VersionLabelName: "7.1.0",
				},
			},
		},
	},
}

func TestESVersionMatch(t *testing.T) {
	require.Equal(t, true,
		ESVersionMatch(ssetv7, func(v version.Version) bool {
			return v.Major == 7
		}),
	)
	require.Equal(t, false,
		ESVersionMatch(ssetv7, func(v version.Version) bool {
			return v.Major == 6
		}),
	)
}

func TestAtLeastOneESVersionMatch(t *testing.T) {
	ssetv6 := *ssetv7.DeepCopy()
	ssetv6.Spec.Template.Labels[label.VersionLabelName] = "6.8.0"

	require.Equal(t, true,
		AtLeastOneESVersionMatch(StatefulSetList{ssetv6, ssetv7}, func(v version.Version) bool {
			return v.Major == 7
		}),
	)
	require.Equal(t, false,
		AtLeastOneESVersionMatch(StatefulSetList{ssetv6, ssetv6}, func(v version.Version) bool {
			return v.Major == 7
		}),
	)
}

func TestStatefulSetList_GetExistingPods(t *testing.T) {
	// 2 pods that belong to the sset
	pod1 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod1",
			Labels: map[string]string{
				label.StatefulSetNameLabelName: ssetv7.Name,
			},
		},
	}
	pod2 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod2",
			Labels: map[string]string{
				label.StatefulSetNameLabelName: ssetv7.Name,
			},
		},
	}
	client := k8s.WrapClient(fake.NewFakeClient(&pod1, &pod2))
	pods, err := StatefulSetList{ssetv7}.GetActualPods(client)
	require.NoError(t, err)
	require.Equal(t, []corev1.Pod{pod1, pod2}, pods)
	// TODO: test with an additional pod that does not belong to the sset and
	//  check it is not returned.
	//  This cannot be done currently since the fake client does not support label list options.
	//  See https://github.com/kubernetes-sigs/controller-runtime/pull/311
}

func TestStatefulSetList_PodReconciliationDone(t *testing.T) {
	es := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cluster"},
	}
	statefulSet1 := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset1"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: common.Int32(2),
		},
	}
	statefulSet2 := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sset2"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: common.Int32(1),
		},
	}
	sset1Pod0 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: "sset1-0", Labels: map[string]string{
			label.ClusterNameLabelName: es.Name,
		}},
	}
	sset1Pod1 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: "sset1-1", Labels: map[string]string{
			label.ClusterNameLabelName: es.Name,
		}},
	}
	sset1Pod2 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: "sset1-2", Labels: map[string]string{
			label.ClusterNameLabelName: es.Name,
		}},
	}
	sset2Pod0 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: "sset2-0", Labels: map[string]string{
			label.ClusterNameLabelName: es.Name,
		}},
	}
	tests := []struct {
		name string
		l    StatefulSetList
		c    k8s.Client
		want bool
	}{
		{
			name: "no pods, no sset",
			l:    nil,
			c:    k8s.WrapClient(fake.NewFakeClient()),
			want: true,
		},
		{
			name: "some pods, no sset",
			l:    nil,
			c:    k8s.WrapClient(fake.NewFakeClient(&sset1Pod0)),
			want: false,
		},
		{
			name: "some statefulSets, no pod",
			l:    StatefulSetList{statefulSet1, statefulSet2},
			c:    k8s.WrapClient(fake.NewFakeClient(&statefulSet1, &statefulSet2)),
			want: false,
		},
		{
			name: "missing sset1Pod1",
			l:    StatefulSetList{statefulSet1, statefulSet2},
			c:    k8s.WrapClient(fake.NewFakeClient(&statefulSet1, &statefulSet2, &sset1Pod0, &sset2Pod0)),
			want: false,
		},
		{
			name: "additional pod sset1Pod2 that shouldn't be there",
			l:    StatefulSetList{statefulSet1, statefulSet2},
			c:    k8s.WrapClient(fake.NewFakeClient(&statefulSet1, &statefulSet2, &sset1Pod0, &sset1Pod1, &sset1Pod2, &sset2Pod0)),
			want: false,
		},
		{
			name: "pods match sset spec",
			l:    StatefulSetList{statefulSet1, statefulSet2},
			c:    k8s.WrapClient(fake.NewFakeClient(&statefulSet1, &statefulSet2, &sset1Pod0, &sset1Pod1, &sset2Pod0)),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.l.PodReconciliationDone(tt.c, es)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestStatefulSetList_GetByName(t *testing.T) {
	sset := func(name string) appsv1.StatefulSet {
		return appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}
	tests := []struct {
		name       string
		l          StatefulSetList
		ssetName   string
		wantResult appsv1.StatefulSet
		wantFound  bool
	}{
		{
			name:      "statefulset not found",
			l:         StatefulSetList{sset("a"), sset("b")},
			ssetName:  "c",
			wantFound: false,
		},
		{
			name:       "statefulset found",
			l:          StatefulSetList{sset("a"), sset("b")},
			ssetName:   "b",
			wantFound:  true,
			wantResult: sset("b"),
		},
		{
			name:      "empty list",
			l:         StatefulSetList{},
			ssetName:  "b",
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found := tt.l.GetByName(tt.ssetName)
			if !reflect.DeepEqual(result, tt.wantResult) {
				t.Errorf("GetByName() got = %v, want %v", result, tt.wantResult)
			}
			if found != tt.wantFound {
				t.Errorf("GetByName() got1 = %v, want %v", found, tt.wantFound)
			}
		})
	}
}

func TestStatefulSetList_ToUpdate(t *testing.T) {
	toUpdate1 := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "toUpdate1"},
		Status:     appsv1.StatefulSetStatus{UpdateRevision: "update-rev", CurrentRevision: "current-rev"},
	}
	toUpdate2 := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "toUpdate2"},
		Status:     appsv1.StatefulSetStatus{UpdateRevision: "update-rev", CurrentRevision: "current-rev"},
	}
	noUpdateRev := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "noUpdateRev"},
		Status:     appsv1.StatefulSetStatus{UpdateRevision: "", CurrentRevision: "current-rev"},
	}
	updateMatchCurrent := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "noUpdateRev"},
		Status:     appsv1.StatefulSetStatus{UpdateRevision: "update-rev", CurrentRevision: "update-rev"},
	}
	tests := []struct {
		name string
		l    StatefulSetList
		want StatefulSetList
	}{
		{
			name: "empty list",
			l:    StatefulSetList{},
			want: StatefulSetList{},
		},
		{
			name: "2/4 StatefulSets to update",
			l:    StatefulSetList{noUpdateRev, toUpdate1, updateMatchCurrent, toUpdate2},
			want: StatefulSetList{toUpdate1, toUpdate2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.l.ToUpdate(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}
