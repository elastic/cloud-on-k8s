// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen2

import (
	"testing"

	"github.com/stretchr/testify/require"

	settings2 "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
)

func TestSetupInitialMasterNodes(t *testing.T) {
	tests := []struct {
		name              string
		nodeSpecResources nodespec.ResourcesList
		expected          []settings.CanonicalConfig
	}{
		{
			name: "no master nodes",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: sset.TestSset{Name: "data", Version: "7.1.0", Replicas: 3, Master: false, Data: true}.Build(), Config: settings.NewCanonicalConfig()},
			},
			expected: []settings.CanonicalConfig{settings.NewCanonicalConfig()},
		},
		{
			name: "3 masters, 3 master+data, 3 data",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: sset.TestSset{Name: "master", Version: "7.1.0", Replicas: 3, Master: true, Data: false}.Build(), Config: settings.NewCanonicalConfig()},
				{StatefulSet: sset.TestSset{Name: "masterdata", Version: "7.1.0", Replicas: 3, Master: true, Data: true}.Build(), Config: settings.NewCanonicalConfig()},
				{StatefulSet: sset.TestSset{Name: "data", Version: "7.1.0", Replicas: 3, Master: false, Data: true}.Build(), Config: settings.NewCanonicalConfig()},
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
			name: "versionCompatibleWithZen2 <7: nothing should appear in the config",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: sset.TestSset{Name: "master", Version: "6.8.0", Replicas: 3, Master: true, Data: false}.Build(), Config: settings.NewCanonicalConfig()},
			},
			expected: []settings.CanonicalConfig{settings.NewCanonicalConfig()},
		},
		{
			name: "mixed v6 & v7: include all masters but only in v7 configs",
			nodeSpecResources: nodespec.ResourcesList{
				{StatefulSet: sset.TestSset{Name: "masterv6", Version: "6.8.0", Replicas: 3, Master: true, Data: false}.Build(), Config: settings.NewCanonicalConfig()},
				{StatefulSet: sset.TestSset{Name: "masterv7", Version: "7.1.0", Replicas: 3, Master: true, Data: false}.Build(), Config: settings.NewCanonicalConfig()},
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
			err := SetupInitialMasterNodes(tt.nodeSpecResources)
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
