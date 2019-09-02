// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen2

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type fakeESClient struct {
	called bool
	client.Client
}

func (f *fakeESClient) DeleteVotingConfigExclusions(ctx context.Context, waitForRemoval bool) error {
	f.called = true
	return nil
}

func Test_ClearVotingConfigExclusions(t *testing.T) {
	// dummy statefulset with 3 pods
	statefulSet3rep := sset.TestSset{Name: "nodes", Version: "7.2.0", Replicas: 3, Master: true, Data: true}.Build()
	es := v1alpha1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "es", Namespace: statefulSet3rep.Namespace}}
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
		actualStatefulSets sset.StatefulSetList
		wantCall           bool
		wantRequeue        bool
	}{
		{
			name: "no v7 nodes",
			c:    k8s.WrapClient(fake.NewFakeClient()),
			actualStatefulSets: sset.StatefulSetList{
				createStatefulSetWithESVersion("6.8.0"),
			},
			wantCall:    false,
			wantRequeue: false,
		},
		{
			name:               "3/3 nodes there: can clear",
			c:                  k8s.WrapClient(fake.NewFakeClient(&statefulSet3rep, &pods[0], &pods[1], &pods[2])),
			actualStatefulSets: sset.StatefulSetList{statefulSet3rep},
			wantCall:           true,
			wantRequeue:        false,
		},
		{
			name:               "2/3 nodes there: cannot clear, should requeue",
			c:                  k8s.WrapClient(fake.NewFakeClient(&statefulSet3rep, &pods[0], &pods[1])),
			actualStatefulSets: sset.StatefulSetList{statefulSet3rep},
			wantCall:           false,
			wantRequeue:        true,
		},
		{
			name:               "3/2 nodes there: cannot clear, should requeue",
			c:                  k8s.WrapClient(fake.NewFakeClient(&statefulSet2rep, &pods[0], &pods[1], &pods[2])),
			actualStatefulSets: sset.StatefulSetList{statefulSet2rep},
			wantCall:           false,
			wantRequeue:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientMock := &fakeESClient{}
			requeue, err := ClearVotingConfigExclusions(es, tt.c, clientMock, tt.actualStatefulSets)
			require.NoError(t, err)
			require.Equal(t, tt.wantRequeue, requeue)
			require.Equal(t, tt.wantCall, clientMock.called)
		})
	}
}
