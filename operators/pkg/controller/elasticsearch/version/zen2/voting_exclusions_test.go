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

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
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
	statefulSet3rep := createStatefulSet("nodes", "7.2.0", 3, true, true)
	pods := make([]corev1.Pod, 0, *statefulSet3rep.Spec.Replicas)
	for _, podName := range sset.PodNames(statefulSet3rep) {
		pods = append(pods, corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace: statefulSet3rep.Namespace,
			Name:      podName,
			Labels: map[string]string{
				label.StatefulSetNameLabelName: statefulSet3rep.Name,
			},
		}})
	}
	// simulate 2 pods out of the 3
	statefulSet2rep := createStatefulSet("nodes", "7.2.0", 2, true, true)
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
			name:               "2/3 nodes there: can clear",
			c:                  k8s.WrapClient(fake.NewFakeClient(&statefulSet3rep, &pods[0], &pods[1])),
			actualStatefulSets: sset.StatefulSetList{statefulSet3rep},
			wantCall:           true,
			wantRequeue:        false,
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
			requeue, err := ClearVotingConfigExclusions(v1alpha1.Elasticsearch{}, tt.c, clientMock, tt.actualStatefulSets)
			require.NoError(t, err)
			require.Equal(t, tt.wantRequeue, requeue)
			require.Equal(t, tt.wantCall, clientMock.called)
		})
	}
}
