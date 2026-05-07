// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package sset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestStatefulSetName(t *testing.T) {
	type args struct {
		podName string
	}
	tests := []struct {
		name         string
		args         args
		wantSsetName string
		wantOrdinal  int32
		wantErr      bool
	}{
		{
			name:         "Get the name of the StatefulSet from a Pod",
			args:         args{podName: "foo-14"},
			wantErr:      false,
			wantOrdinal:  14,
			wantSsetName: "foo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSsetName, gotOrdinal, err := StatefulSetName(tt.args.podName)
			if (err != nil) != tt.wantErr {
				t.Errorf("StatefulSetName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotSsetName != tt.wantSsetName {
				t.Errorf("StatefulSetName() gotSsetName = %v, want %v", gotSsetName, tt.wantSsetName)
			}
			if gotOrdinal != tt.wantOrdinal {
				t.Errorf("StatefulSetName() gotOrdinal = %v, want %v", gotOrdinal, tt.wantOrdinal)
			}
		})
	}
}

// Test that we actually filter on the sset name and the namespace
func TestGetActualPodsForStatefulSet(t *testing.T) {
	objs := []client.Object{
		getPodSample("pod0", "ns0", "sset0", "clus0", "0"),
		getPodSample("pod1", "ns1", "sset0", "clus0", "0"),
		getPodSample("pod2", "ns0", "sset1", "clus1", "0"),
		getPodSample("pod3", "ns0", "sset1", "clus0", "0"),
	}
	c := k8s.NewFakeClient(objs...)
	sset0 := getSsetSample("sset0", "ns0", "clus0")
	pods, err := GetActualPodsForStatefulSet(c, k8s.ExtractNamespacedName(&sset0))
	require.NoError(t, err)
	// only one pod is in the same stateful set and namespace
	assert.Equal(t, 1, len(pods))
}

func TestGetActualMastersForCluster(t *testing.T) {
	masterPod := sset.TestPod{
		Namespace:       "ns0",
		Name:            "pod0",
		ClusterName:     "clus0",
		StatefulSetName: "sset0",
		Master:          true,
	}.BuildPtr()

	objs := []client.Object{
		masterPod,
		getPodSample("pod1", "ns0", "sset0", "clus0", "0"),
		getPodSample("pod2", "ns1", "sset0", "clus0", "0"),
		getPodSample("pod3", "ns0", "sset1", "clus1", "0"),
		getPodSample("pod4", "ns0", "sset1", "clus0", "0"),
	}
	c := k8s.NewFakeClient(objs...)

	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name: "clus0",
		},
	}

	masters, err := GetActualMastersForCluster(c, es)
	require.NoError(t, err)
	require.Len(t, masters, 1)
	require.Equal(t, "pod0", masters[0].GetName())
}

func TestGetActualPodsRestartTriggerAnnotationForCluster(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clus0",
			Namespace: "ns0",
		},
	}

	tests := []struct {
		name    string
		objs    []client.Object
		want    string
		wantErr bool
	}{
		{
			name: "no pods in the cluster",
			objs: nil,
			want: "",
		},
		{
			name: "single pod with no annotations",
			objs: []client.Object{
				getPodSample("pod0", "ns0", "sset0", "clus0", "0"),
			},
			want: "",
		},
		{
			name: "single pod with restart-trigger annotation",
			objs: []client.Object{
				withAnnotations(
					getPodSample("pod0", "ns0", "sset0", "clus0", "0"),
					map[string]string{esv1.RestartTriggerAnnotation: "2026-01-14T12:00:00Z"},
				),
			},
			want: "2026-01-14T12:00:00Z",
		},
		{
			name: "multiple pods, all with same annotation",
			objs: []client.Object{
				withAnnotations(
					getPodSample("pod0", "ns0", "sset0", "clus0", "0"),
					map[string]string{esv1.RestartTriggerAnnotation: "2026-01-14T12:00:00Z"},
				),
				withAnnotations(
					getPodSample("pod1", "ns0", "sset0", "clus0", "0"),
					map[string]string{esv1.RestartTriggerAnnotation: "2026-01-14T12:00:00Z"},
				),
			},
			want: "2026-01-14T12:00:00Z",
		},
		{
			name: "multiple pods, returns highest annotation value",
			objs: []client.Object{
				withAnnotations(
					getPodSample("pod0", "ns0", "sset0", "clus0", "0"),
					map[string]string{esv1.RestartTriggerAnnotation: "2026-01-01T00:00:00Z"},
				),
				withAnnotations(
					getPodSample("pod1", "ns0", "sset0", "clus0", "0"),
					map[string]string{esv1.RestartTriggerAnnotation: "2026-06-15T00:00:00Z"},
				),
				withAnnotations(
					getPodSample("pod2", "ns0", "sset0", "clus0", "0"),
					map[string]string{esv1.RestartTriggerAnnotation: "2026-03-10T00:00:00Z"},
				),
			},
			want: "2026-06-15T00:00:00Z",
		},
		{
			name: "mix of pods with and without annotation",
			objs: []client.Object{
				getPodSample("pod0", "ns0", "sset0", "clus0", "0"),
				withAnnotations(
					getPodSample("pod1", "ns0", "sset0", "clus0", "0"),
					map[string]string{esv1.RestartTriggerAnnotation: "trigger-v1"},
				),
			},
			want: "trigger-v1",
		},
		{
			name: "pods in different namespace are ignored",
			objs: []client.Object{
				withAnnotations(
					getPodSample("pod0", "ns0", "sset0", "clus0", "0"),
					map[string]string{esv1.RestartTriggerAnnotation: "a"},
				),
				withAnnotations(
					getPodSample("pod1", "ns1", "sset0", "clus0", "0"),
					map[string]string{esv1.RestartTriggerAnnotation: "z"},
				),
			},
			want: "a",
		},
		{
			name: "pods in different cluster are ignored",
			objs: []client.Object{
				withAnnotations(
					getPodSample("pod0", "ns0", "sset0", "clus0", "0"),
					map[string]string{esv1.RestartTriggerAnnotation: "a"},
				),
				withAnnotations(
					getPodSample("pod1", "ns0", "sset0", "clus1", "0"),
					map[string]string{esv1.RestartTriggerAnnotation: "z"},
				),
			},
			want: "a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.objs...)
			got, err := GetActualPodsRestartTriggerAnnotationForCluster(c, es)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func withAnnotations(pod *corev1.Pod, annotations map[string]string) *corev1.Pod {
	pod.Annotations = annotations
	return pod
}

func getSsetSample(name, namespace, clusterName string) appsv1.StatefulSet {
	return sset.TestSset{
		Name:        name,
		Namespace:   namespace,
		ClusterName: clusterName,
		Replicas:    3,
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "1",
			UpdateRevision:  "1",
		},
	}.Build()
}

func getPodSample(name, namespace, ssetName, clusterName, revision string) *corev1.Pod {
	return sset.TestPod{
		Namespace:       namespace,
		Name:            name,
		ClusterName:     clusterName,
		StatefulSetName: ssetName,
		Revision:        revision,
	}.BuildPtr()
}
