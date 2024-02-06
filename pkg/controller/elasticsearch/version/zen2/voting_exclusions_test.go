// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package zen2

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	es_sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

type fakeVotingConfigExclusionsESClient struct {
	called        bool
	excludedNodes []string
	client.Client
}

func (f *fakeVotingConfigExclusionsESClient) DeleteVotingConfigExclusions(_ context.Context, _ bool) error {
	f.called = true
	return nil
}

func (f *fakeVotingConfigExclusionsESClient) AddVotingConfigExclusions(_ context.Context, nodeNames []string) error {
	f.called = true
	f.excludedNodes = nodeNames
	return nil
}

func Test_ClearVotingConfigExclusions(t *testing.T) {
	// dummy statefulset with 3 pods
	statefulSet3rep := sset.TestSset{Name: "nodes", Version: "7.2.0", Replicas: 3, Master: true, Data: true}.Build()
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "es", Namespace: statefulSet3rep.Namespace}}
	pods := make([]corev1.Pod, 0, *statefulSet3rep.Spec.Replicas)
	for _, podName := range sset.PodNames(statefulSet3rep) {
		pods = append(pods, sset.TestPod{
			Namespace:       statefulSet3rep.Namespace,
			Name:            podName,
			ClusterName:     es.Name,
			Version:         "7.2.0",
			Master:          true,
			StatefulSetName: statefulSet3rep.Name,
		}.Build())
	}
	// simulate 2 pods out of the 3
	statefulSet2rep := sset.TestSset{Name: "nodes", Version: "7.2.0", Replicas: 2, Master: true, Data: true}.Build()
	tests := []struct {
		name               string
		c                  k8s.Client
		es                 *esv1.Elasticsearch
		actualStatefulSets es_sset.StatefulSetList
		wantCall           bool
		wantRequeue        bool
	}{
		{
			name: "no v7 nodes",
			c:    k8s.NewFakeClient(&es),
			es:   &es,
			actualStatefulSets: es_sset.StatefulSetList{
				createStatefulSetWithESVersion("6.8.0"),
			},
			wantCall:    false,
			wantRequeue: false,
		},
		{
			name:               "3/3 nodes there, should clear",
			c:                  k8s.NewFakeClient(&es, &statefulSet3rep, &pods[0], &pods[1], &pods[2]),
			es:                 &es,
			actualStatefulSets: es_sset.StatefulSetList{statefulSet3rep},
			wantCall:           true,
			wantRequeue:        false,
		},
		{
			name:               "2/3 nodes there: cannot clear, should requeue",
			c:                  k8s.NewFakeClient(&es, &statefulSet3rep, &pods[0], &pods[1]),
			es:                 &es,
			actualStatefulSets: es_sset.StatefulSetList{statefulSet3rep},
			wantCall:           false,
			wantRequeue:        true,
		},
		{
			name:               "3/2 nodes there: cannot clear, should requeue",
			es:                 &es,
			c:                  k8s.NewFakeClient(&es, &statefulSet2rep, &pods[0], &pods[1], &pods[2]),
			actualStatefulSets: es_sset.StatefulSetList{statefulSet2rep},
			wantCall:           false,
			wantRequeue:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientMock := &fakeVotingConfigExclusionsESClient{}
			requeue, err := ClearVotingConfigExclusions(context.Background(), *tt.es, tt.c, clientMock, tt.actualStatefulSets)
			require.NoError(t, err)
			require.Equal(t, tt.wantRequeue, requeue)
			require.Equal(t, tt.wantCall, clientMock.called)
			var retrievedES esv1.Elasticsearch
			err = tt.c.Get(context.Background(), k8s.ExtractNamespacedName(tt.es), &retrievedES)
			require.NoError(t, err)
		})
	}
}

func TestAddToVotingConfigExclusions(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "es", Namespace: "ns"}}
	masterPod := sset.TestPod{
		Namespace:   "ns",
		Name:        "pod-name",
		ClusterName: "es",
		Version:     "7.2.0",
		Master:      true,
	}.BuildPtr()
	tests := []struct {
		name              string
		es                *esv1.Elasticsearch
		c                 k8s.Client
		excludeNodes      []string
		wantAPICalled     bool
		wantAPICalledWith []string
	}{
		{
			name: "some zen1 masters: do nothing",
			es:   &es,
			c: k8s.NewFakeClient(&es, sset.TestPod{
				Namespace:   "ns",
				Name:        "pod-name",
				ClusterName: "es",
				Version:     "6.8.0",
				Master:      true,
			}.BuildPtr()),
			excludeNodes:  []string{"node1"},
			wantAPICalled: false,
		},
		{
			name:              "set voting config exclusions",
			es:                &es,
			c:                 k8s.NewFakeClient(&es, masterPod),
			excludeNodes:      []string{"node1", "node2"},
			wantAPICalled:     true,
			wantAPICalledWith: []string{"node1", "node2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientMock := &fakeVotingConfigExclusionsESClient{}
			err := AddToVotingConfigExclusions(context.Background(), tt.c, clientMock, *tt.es, tt.excludeNodes)
			require.NoError(t, err)
			require.Equal(t, tt.wantAPICalled, clientMock.called)
			require.Equal(t, tt.wantAPICalledWith, clientMock.excludedNodes)
			var retrievedES esv1.Elasticsearch
			err = tt.c.Get(context.Background(), k8s.ExtractNamespacedName(tt.es), &retrievedES)
			require.NoError(t, err)
		})
	}
}
