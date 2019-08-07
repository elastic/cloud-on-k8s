// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	settings2 "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

const (
	defaultClusterUUID = "jiMyMA1hQ-WMPK3vEStZuw"
)

func setupScheme(t *testing.T) *runtime.Scheme {
	sc := scheme.Scheme
	if err := v1alpha1.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to add Es types")
	}
	return sc
}

var esNN = types.NamespacedName{
	Namespace: "ns1",
	Name:      "foo",
}

func newElasticsearch() v1alpha1.Elasticsearch {
	return v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esNN.Namespace,
			Name:      esNN.Name,
		},
	}
}

func withAnnotation(es v1alpha1.Elasticsearch, name, value string) v1alpha1.Elasticsearch {
	if es.Annotations == nil {
		es.Annotations = make(map[string]string)
	}
	es.Annotations[name] = value
	return es
}

func TestSetupInitialMasterNodes_AlreadyBootstrapped(t *testing.T) {
	s := setupScheme(t)
	tests := []struct {
		name              string
		es                v1alpha1.Elasticsearch
		observedState     observer.State
		nodeSpecResources nodespec.ResourcesList
		expected          []settings.CanonicalConfig
		expectedEs        v1alpha1.Elasticsearch
	}{
		{
			name: "cluster already annotated for bootstrap: no changes",
			es:   withAnnotation(newElasticsearch(), ClusterUUIDAnnotationName, defaultClusterUUID),
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: nodespec.CreateTestSset("data", "7.1.0", 3, false, true), Config: settings.NewCanonicalConfig()},
			},
			expected:   []settings.CanonicalConfig{settings.NewCanonicalConfig()},
			expectedEs: withAnnotation(newElasticsearch(), ClusterUUIDAnnotationName, defaultClusterUUID),
		},
		{
			name:          "cluster bootstrapped but not annotated: should be annotated",
			es:            newElasticsearch(),
			observedState: observer.State{ClusterState: &client.ClusterState{ClusterUUID: defaultClusterUUID}},
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: nodespec.CreateTestSset("data", "7.1.0", 3, false, true), Config: settings.NewCanonicalConfig()},
			},
			expected:   []settings.CanonicalConfig{settings.NewCanonicalConfig()},
			expectedEs: withAnnotation(newElasticsearch(), ClusterUUIDAnnotationName, defaultClusterUUID),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.WrapClient(fake.NewFakeClientWithScheme(s, &tt.es))
			err := SetupInitialMasterNodes(tt.es, tt.observedState, client, tt.nodeSpecResources)
			require.NoError(t, err)
			// check if the ES resource was annotated
			var es v1alpha1.Elasticsearch
			err = client.Get(esNN, &es)
			assert.NoError(t, err)
			require.Equal(t, tt.expectedEs, es)
			// check if nodespec config were modified
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

func TestSetupInitialMasterNodes_NotBootstrapped(t *testing.T) {
	tests := []struct {
		name              string
		nodeSpecResources nodespec.ResourcesList
		expected          []settings.CanonicalConfig
	}{
		{
			name: "no master nodes",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: nodespec.CreateTestSset("data", "7.1.0", 3, false, true), Config: settings.NewCanonicalConfig()},
			},
			expected: []settings.CanonicalConfig{settings.NewCanonicalConfig()},
		},
		{
			name: "3 masters, 3 master+data, 3 data",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: nodespec.CreateTestSset("master", "7.1.0", 3, true, false), Config: settings.NewCanonicalConfig()},
				{StatefulSet: nodespec.CreateTestSset("masterdata", "7.1.0", 3, true, true), Config: settings.NewCanonicalConfig()},
				{StatefulSet: nodespec.CreateTestSset("data", "7.1.0", 3, false, true), Config: settings.NewCanonicalConfig()},
			},
			expected: []settings.CanonicalConfig{
				{CanonicalConfig: settings2.MustCanonicalConfig(map[string][]string{
					settings.ClusterInitialMasterNodes: {"master-0", "master-1", "master-2", "masterdata-0", "masterdata-1", "masterdata-2"},
				})},
				{CanonicalConfig: settings2.MustCanonicalConfig(map[string][]string{
					settings.ClusterInitialMasterNodes: {"master-0", "master-1", "master-2", "masterdata-0", "masterdata-1", "masterdata-2"},
				})},
				// no config set on non-master nodes
				{CanonicalConfig: settings2.NewCanonicalConfig()},
			},
		},
		{
			name: "version <7: nothing should appear in the config",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: nodespec.CreateTestSset("master", "6.8.0", 3, true, false), Config: settings.NewCanonicalConfig()},
			},
			expected: []settings.CanonicalConfig{settings.NewCanonicalConfig()},
		},
		{
			name: "mixed v6 & v7: include all masters but only in v7 configs",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: nodespec.CreateTestSset("masterv6", "6.8.0", 3, true, false), Config: settings.NewCanonicalConfig()},
				{StatefulSet: nodespec.CreateTestSset("masterv7", "7.1.0", 3, true, false), Config: settings.NewCanonicalConfig()},
			},
			expected: []settings.CanonicalConfig{
				settings.NewCanonicalConfig(),
				{CanonicalConfig: settings2.MustCanonicalConfig(map[string][]string{
					settings.ClusterInitialMasterNodes: {"masterv6-0", "masterv6-1", "masterv6-2", "masterv7-0", "masterv7-1", "masterv7-2"},
				})},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetupInitialMasterNodes(v1alpha1.Elasticsearch{}, observer.State{}, k8s.WrapClient(fake.NewFakeClient()), tt.nodeSpecResources)
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
