// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen1

import (
	"context"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	settings2 "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSetupMinimumMasterNodesConfig(t *testing.T) {
	tests := []struct {
		name              string
		nodeSpecResources nodespec.ResourcesList
		expected          []settings.CanonicalConfig
		pods              []runtime.Object
	}{
		{
			name: "no master nodes",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: sset.TestSset{Name: "data", Version: "7.1.0", Replicas: 3, Master: false, Data: true}.Build(), Config: settings.NewCanonicalConfig()},
			},
			expected: []settings.CanonicalConfig{settings.NewCanonicalConfig()},
			pods:     createMasterPodsWithVersion("data", "7.1.0", 3),
		},
		{
			name: "3 masters, 3 master+data, 3 data",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: sset.TestSset{Name: "master", Version: "6.8.0", Replicas: 3, Master: true, Data: false}.Build(), Config: settings.NewCanonicalConfig()},
				{StatefulSet: sset.TestSset{Name: "masterdata", Version: "6.8.0", Replicas: 3, Master: true, Data: true}.Build(), Config: settings.NewCanonicalConfig()},
				{StatefulSet: sset.TestSset{Name: "data", Version: "6.8.0", Replicas: 3, Master: false, Data: true}.Build(), Config: settings.NewCanonicalConfig()},
			},
			expected: []settings.CanonicalConfig{
				{CanonicalConfig: settings2.MustCanonicalConfig(map[string]string{
					settings.DiscoveryZenMinimumMasterNodes: "4",
				})},
				{CanonicalConfig: settings2.MustCanonicalConfig(map[string]string{
					settings.DiscoveryZenMinimumMasterNodes: "4",
				})},
				{CanonicalConfig: settings2.MustCanonicalConfig(map[string]string{
					settings.DiscoveryZenMinimumMasterNodes: "4",
				})},
			},
			pods: []runtime.Object{},
		},
		{
			name: "v7 in the spec but still have some 6.x in flight",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: sset.TestSset{Name: "masterv7", Version: "7.1.0", Replicas: 3, Master: true, Data: false}.Build(), Config: settings.NewCanonicalConfig()},
			},
			expected: []settings.CanonicalConfig{
				{CanonicalConfig: settings2.MustCanonicalConfig(map[string]string{
					settings.DiscoveryZenMinimumMasterNodes: "2",
				})},
				settings.NewCanonicalConfig(),
			},
			pods: createMasterPodsWithVersion("data", "6.8.0", 3),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.WrapClient(fake.NewFakeClient(tt.pods...))
			err := SetupMinimumMasterNodesConfig(client, testES, tt.nodeSpecResources)
			require.NoError(t, err)
			for i := 0; i < len(tt.nodeSpecResources); i++ {
				expected, err := tt.expected[i].Render()
				require.NoError(t, err)
				actual, err := tt.nodeSpecResources[i].Config.Render()
				require.NoError(t, err)
				require.Equal(t, expected, actual)
			}
		})
	}
}

type fakeESClient struct {
	called     bool
	calledWith int
	client.Client
}

func (f *fakeESClient) SetMinimumMasterNodes(ctx context.Context, count int) error {
	f.called = true
	f.calledWith = count
	return nil
}

func TestUpdateMinimumMasterNodes(t *testing.T) {
	esName := "es"
	ns := "ns"
	ssetSample := sset.TestSset{Name: "nodes", Namespace: ns, ClusterName: esName, Version: "6.8.0", Replicas: 3, Master: true, Data: true}.Build()
	// simulate 3/3 pods ready
	labels := map[string]string{
		label.StatefulSetNameLabelName: ssetSample.Name,
		label.VersionLabelName:         "6.8.0",
		label.ClusterNameLabelName:     esName,
	}
	label.NodeTypesMasterLabelName.Set(true, labels)
	label.NodeTypesDataLabelName.Set(true, labels)
	podsReady3 := make([]corev1.Pod, 0, 3)
	for _, podName := range sset.PodNames(ssetSample) {
		podsReady3 = append(podsReady3, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ssetSample.Namespace,
				Name:      podName,
				Labels:    labels,
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Status: corev1.ConditionTrue,
						Type:   corev1.ContainersReady,
					},
					{
						Status: corev1.ConditionTrue,
						Type:   corev1.PodReady,
					},
				},
			},
		})
	}
	// simulate 1/3 pods ready
	podsReady1 := make([]corev1.Pod, 3)
	podsReady1[0] = *podsReady3[0].DeepCopy()
	podsReady1[0].Status.Conditions[0].Status = corev1.ConditionFalse
	podsReady1[1] = *podsReady3[1].DeepCopy()
	podsReady1[1].Status.Conditions[0].Status = corev1.ConditionFalse
	podsReady1[2] = *podsReady3[2].DeepCopy()

	tests := []struct {
		wantCalled         bool
		wantRequeue        bool
		wantCalledWith     int
		c                  k8s.Client
		name               string
		actualStatefulSets sset.StatefulSetList
		reconcileState     *reconcile.State
	}{
		{
			name:               "no v6 nodes",
			actualStatefulSets: sset.StatefulSetList{sset.TestSset{Name: "nodes", Namespace: ns, Version: "7.1.0", Replicas: 3, Master: true, Data: true}.Build()},
			wantCalled:         false,
			c:                  k8s.WrapClient(fake.NewFakeClient(createMasterPodsWithVersion("nodes", "7.1.0", 3)...)),
		},
		{
			name:               "correct mmn already set in ES status",
			c:                  k8s.WrapClient(fake.NewFakeClient(&podsReady3[0], &podsReady3[1], &podsReady3[2])),
			actualStatefulSets: sset.StatefulSetList{ssetSample},
			reconcileState:     reconcile.NewState(v1beta1.Elasticsearch{Status: v1beta1.ElasticsearchStatus{ZenDiscovery: v1beta1.ZenDiscoveryStatus{MinimumMasterNodes: 2}}}),
			wantCalled:         false,
		},
		{
			name:               "mmn should be updated, it's different in the ES status",
			c:                  k8s.WrapClient(fake.NewFakeClient(&podsReady3[0], &podsReady3[1], &podsReady3[2])),
			actualStatefulSets: sset.StatefulSetList{ssetSample},
			reconcileState:     reconcile.NewState(v1beta1.Elasticsearch{Status: v1beta1.ElasticsearchStatus{ZenDiscovery: v1beta1.ZenDiscoveryStatus{MinimumMasterNodes: 1}}}),
			wantCalled:         true,
			wantCalledWith:     2,
		},
		{
			name:               "mmn should be updated, it isn't set in the ES status",
			c:                  k8s.WrapClient(fake.NewFakeClient(&podsReady3[0], &podsReady3[1], &podsReady3[2])),
			actualStatefulSets: sset.StatefulSetList{ssetSample},
			reconcileState:     reconcile.NewState(v1beta1.Elasticsearch{}),
			wantCalled:         true,
			wantCalledWith:     2,
		},
		{
			name:               "cannot update since not enough masters available",
			c:                  k8s.WrapClient(fake.NewFakeClient(&podsReady1[0], &podsReady1[1], &podsReady1[2])),
			actualStatefulSets: sset.StatefulSetList{ssetSample},
			reconcileState:     reconcile.NewState(v1beta1.Elasticsearch{}),
			wantCalled:         false,
			wantRequeue:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			esClient := &fakeESClient{}
			requeue, err := UpdateMinimumMasterNodes(tt.c, v1beta1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: esName, Namespace: ns}}, esClient, tt.actualStatefulSets, tt.reconcileState)
			require.NoError(t, err)
			require.Equal(t, tt.wantRequeue, requeue)
			require.Equal(t, tt.wantCalled, esClient.called)
			require.Equal(t, tt.wantCalledWith, esClient.calledWith)
		})
	}
}
