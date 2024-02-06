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

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
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
