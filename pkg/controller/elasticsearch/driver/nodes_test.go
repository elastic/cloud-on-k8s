// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

func Test_expectedResources_zen2(t *testing.T) {
	es := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "cluster",
		},
		Spec: v1alpha1.ElasticsearchSpec{
			Version: "7.2.0",
			Nodes: []v1alpha1.NodeSpec{
				{
					Name:      "masters",
					NodeCount: 3,
				},
				{
					Name:      "data",
					NodeCount: 3,
				},
			},
		},
	}

	resources, err := expectedResources(k8s.WrapClient(fake.NewFakeClient()), es, observer.State{}, nil)
	require.NoError(t, err)

	// 2 StatefulSets should be created
	require.Equal(t, 2, len(resources.StatefulSets()))
	// master nodes config should be patched to account for zen2 initial master nodes
	require.NotEmpty(t, resources[0].Config.HasKeys([]string{settings.ClusterInitialMasterNodes}))
	// zen1 m_m_n specific config should not be set
	require.Empty(t, resources[0].Config.HasKeys([]string{settings.DiscoveryZenMinimumMasterNodes}))
}

func Test_expectedResources_zen1(t *testing.T) {
	es := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "cluster",
		},
		Spec: v1alpha1.ElasticsearchSpec{
			Version: "6.8.0",
			Nodes: []v1alpha1.NodeSpec{
				{
					Name:      "masters",
					NodeCount: 3,
				},
				{
					Name:      "data",
					NodeCount: 3,
				},
			},
		},
	}

	resources, err := expectedResources(k8s.WrapClient(fake.NewFakeClient()), es, observer.State{}, nil)
	require.NoError(t, err)

	// 2 StatefulSets should be created
	require.Equal(t, 2, len(resources.StatefulSets()))
	// master nodes config should be patched to account for zen1 minimum_master_nodes
	require.NotEmpty(t, resources[0].Config.HasKeys([]string{settings.DiscoveryZenMinimumMasterNodes}))
	// zen2 config should not be set
	require.Empty(t, resources[0].Config.HasKeys([]string{settings.ClusterInitialMasterNodes}))
}
