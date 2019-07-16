// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen1

import (
	"context"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	settings2 "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func createStatefulSet(name string, esversion string, replicas int32, master bool, data bool) appsv1.StatefulSet {
	labels := map[string]string{
		label.VersionLabelName: esversion,
	}
	label.NodeTypesMasterLabelName.Set(master, labels)
	label.NodeTypesDataLabelName.Set(data, labels)
	return appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
			},
		},
	}
}

func TestSetupMinimumMasterNodesConfig(t *testing.T) {
	tests := []struct {
		name              string
		nodeSpecResources nodespec.ResourcesList
		expected          []settings.CanonicalConfig
	}{
		{
			name: "no master nodes",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: createStatefulSet("data", "7.1.0", 3, false, true), Config: settings.NewCanonicalConfig()},
			},
			expected: []settings.CanonicalConfig{settings.NewCanonicalConfig()},
		},
		{
			name: "3 masters, 3 master+data, 3 data",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: createStatefulSet("master", "6.8.0", 3, true, false), Config: settings.NewCanonicalConfig()},
				{StatefulSet: createStatefulSet("masterdata", "6.8.0", 3, true, true), Config: settings.NewCanonicalConfig()},
				{StatefulSet: createStatefulSet("data", "6.8.0", 3, false, true), Config: settings.NewCanonicalConfig()},
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
		},
		{
			name: "version 7: nothing should appear in the config",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: createStatefulSet("master", "7.1.0", 3, true, false), Config: settings.NewCanonicalConfig()},
			},
			expected: []settings.CanonicalConfig{settings.NewCanonicalConfig()},
		},
		{
			name: "mixed v6 & v7: include all masters but only in v6 configs",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: createStatefulSet("masterv6", "6.8.0", 3, true, false), Config: settings.NewCanonicalConfig()},
				{StatefulSet: createStatefulSet("masterv7", "7.1.0", 3, true, false), Config: settings.NewCanonicalConfig()},
			},
			expected: []settings.CanonicalConfig{
				{CanonicalConfig: settings2.MustCanonicalConfig(map[string]string{
					settings.DiscoveryZenMinimumMasterNodes: "4",
				})},
				settings.NewCanonicalConfig(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetupMinimumMasterNodesConfig(tt.nodeSpecResources)
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
	ssetSample := createStatefulSet("nodes", "6.8.0", 3, true, true)
	// simulate 3/3 pods ready
	labels := map[string]string{
		label.StatefulSetNameLabelName: ssetSample.Name,
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
		name               string
		c                  k8s.Client
		actualStatefulSets sset.StatefulSetList
		reconcileState     *reconcile.State
		wantCalled         bool
		wantCalledWith     int
		wantRequeue        bool
	}{
		{
			name:               "no v6 nodes",
			actualStatefulSets: sset.StatefulSetList{createStatefulSet("nodes", "7.1.0", 3, true, true)},
			wantCalled:         false,
		},
		{
			name:               "correct mmn already set in ES status",
			c:                  k8s.WrapClient(fake.NewFakeClient(&podsReady3[0], &podsReady3[1], &podsReady3[2])),
			actualStatefulSets: sset.StatefulSetList{ssetSample},
			reconcileState:     reconcile.NewState(v1alpha1.Elasticsearch{Status: v1alpha1.ElasticsearchStatus{ZenDiscovery: v1alpha1.ZenDiscoveryStatus{MinimumMasterNodes: 2}}}),
			wantCalled:         false,
		},
		{
			name:               "mmn should be updated, it's different in the ES status",
			c:                  k8s.WrapClient(fake.NewFakeClient(&podsReady3[0], &podsReady3[1], &podsReady3[2])),
			actualStatefulSets: sset.StatefulSetList{ssetSample},
			reconcileState:     reconcile.NewState(v1alpha1.Elasticsearch{Status: v1alpha1.ElasticsearchStatus{ZenDiscovery: v1alpha1.ZenDiscoveryStatus{MinimumMasterNodes: 1}}}),
			wantCalled:         true,
			wantCalledWith:     2,
		},
		{
			name:               "mmn should be updated, it isn't set in the ES status",
			c:                  k8s.WrapClient(fake.NewFakeClient(&podsReady3[0], &podsReady3[1], &podsReady3[2])),
			actualStatefulSets: sset.StatefulSetList{ssetSample},
			reconcileState:     reconcile.NewState(v1alpha1.Elasticsearch{}),
			wantCalled:         true,
			wantCalledWith:     2,
		},
		{
			name:               "cannot update since not enough masters available",
			c:                  k8s.WrapClient(fake.NewFakeClient(&podsReady1[0], &podsReady1[1], &podsReady1[2])),
			actualStatefulSets: sset.StatefulSetList{ssetSample},
			reconcileState:     reconcile.NewState(v1alpha1.Elasticsearch{}),
			wantCalled:         false,
			wantRequeue:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			esClient := &fakeESClient{}
			requeue, err := UpdateMinimumMasterNodes(tt.c, v1alpha1.Elasticsearch{}, esClient, tt.actualStatefulSets, tt.reconcileState)
			require.NoError(t, err)
			require.Equal(t, tt.wantRequeue, requeue)
			require.Equal(t, tt.wantCalled, esClient.called)
			require.Equal(t, tt.wantCalledWith, esClient.calledWith)
		})
	}
}
