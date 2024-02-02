// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package zen1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	controllerscheme "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/scheme"
	settings2 "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/settings"
	es_sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestSetupMinimumMasterNodesConfig(t *testing.T) {
	tests := []struct {
		name              string
		nodeSpecResources nodespec.ResourcesList
		expected          []settings.CanonicalConfig
		pods              []crclient.Object
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
					esv1.DiscoveryZenMinimumMasterNodes: "4",
				})},
				{CanonicalConfig: settings2.MustCanonicalConfig(map[string]string{
					esv1.DiscoveryZenMinimumMasterNodes: "4",
				})},
				{CanonicalConfig: settings2.MustCanonicalConfig(map[string]string{
					esv1.DiscoveryZenMinimumMasterNodes: "4",
				})},
			},
			pods: []crclient.Object{},
		},
		{
			name: "v7 in the spec but still have some 6.x in flight",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: sset.TestSset{Name: "masterv7", Version: "7.1.0", Replicas: 3, Master: true, Data: false}.Build(), Config: settings.NewCanonicalConfig()},
			},
			expected: []settings.CanonicalConfig{
				{CanonicalConfig: settings2.MustCanonicalConfig(map[string]string{
					esv1.DiscoveryZenMinimumMasterNodes: "2",
				})},
				settings.NewCanonicalConfig(),
			},
			pods: createMasterPodsWithVersion("data", "6.8.0", 3),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.NewFakeClient(tt.pods...)
			err := SetupMinimumMasterNodesConfig(context.Background(), client, testES, tt.nodeSpecResources)
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

func (f *fakeESClient) SetMinimumMasterNodes(_ context.Context, count int) error {
	f.called = true
	f.calledWith = count
	return nil
}

func TestUpdateMinimumMasterNodes(t *testing.T) {
	controllerscheme.SetupScheme()
	esName := "es"
	ns := "ns"
	nsn := types.NamespacedName{Name: esName, Namespace: ns}
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
		es                 esv1.Elasticsearch
		name               string
		actualStatefulSets es_sset.StatefulSetList
	}{
		{
			name:               "no v6 nodes",
			actualStatefulSets: es_sset.StatefulSetList{sset.TestSset{Name: "nodes", Namespace: ns, Version: "7.1.0", Replicas: 3, Master: true, Data: true}.Build()},
			es:                 esv1.Elasticsearch{ObjectMeta: k8s.ToObjectMeta(nsn)},
			wantCalled:         false,
			c:                  k8s.NewFakeClient(createMasterPodsWithVersion("nodes", "7.1.0", 3)...),
		},
		{
			name:               "mmn should be updated",
			c:                  k8s.NewFakeClient(&podsReady3[0], &podsReady3[1], &podsReady3[2]),
			actualStatefulSets: es_sset.StatefulSetList{ssetSample},
			es:                 esv1.Elasticsearch{ObjectMeta: k8s.ToObjectMeta(nsn)},
			wantCalled:         true,
			wantCalledWith:     2,
		},
		{
			name:               "cannot update since not enough masters available",
			c:                  k8s.NewFakeClient(&podsReady1[0], &podsReady1[1], &podsReady1[2]),
			actualStatefulSets: es_sset.StatefulSetList{ssetSample},
			es:                 esv1.Elasticsearch{ObjectMeta: k8s.ToObjectMeta(nsn)},
			wantCalled:         false,
			wantRequeue:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.c.Create(context.Background(), &tt.es))
			esClient := &fakeESClient{}
			requeue, err := UpdateMinimumMasterNodes(context.Background(), tt.c, tt.es, esClient, tt.actualStatefulSets)
			require.NoError(t, err)
			require.Equal(t, tt.wantRequeue, requeue)
			require.Equal(t, tt.wantCalled, esClient.called)
			require.Equal(t, tt.wantCalledWith, esClient.calledWith)
		})
	}
}
