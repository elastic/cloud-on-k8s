// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package autoscaler

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/resources"
	"github.com/stretchr/testify/assert"
)

func TestFairNodesManager_AddNode(t *testing.T) {
	type fields struct {
		nodeSetNodeCountList []resources.NodeSetNodeCount
	}
	tests := []struct {
		name       string
		fields     fields
		assertFunc func(t *testing.T, fnm FairNodesManager)
	}{
		{
			name: "One nodeSet",
			fields: fields{
				nodeSetNodeCountList: []resources.NodeSetNodeCount{{Name: "nodeset-1"}},
			},
			assertFunc: func(t *testing.T, fnm FairNodesManager) {
				assert.Equal(t, 1, len(fnm.nodeSetNodeCountList))
				assert.Equal(t, int32(0), fnm.nodeSetNodeCountList[0].NodeCount)
				fnm.AddNode()
				assert.Equal(t, int32(1), fnm.nodeSetNodeCountList[0].NodeCount)
				fnm.AddNode()
				assert.Equal(t, int32(2), fnm.nodeSetNodeCountList[0].NodeCount)
			},
		},
		{
			name: "Several NodeSets",
			fields: fields{
				nodeSetNodeCountList: []resources.NodeSetNodeCount{{Name: "nodeset-1"}, {Name: "nodeset-2"}},
			},
			assertFunc: func(t *testing.T, fnm FairNodesManager) {
				assert.Equal(t, 2, len(fnm.nodeSetNodeCountList))
				assert.Equal(t, int32(0), fnm.nodeSetNodeCountList.ByNodeSet()["nodeset-1"])
				assert.Equal(t, int32(0), fnm.nodeSetNodeCountList.ByNodeSet()["nodeset-2"])

				fnm.AddNode()
				assert.Equal(t, int32(1), fnm.nodeSetNodeCountList.ByNodeSet()["nodeset-1"])
				assert.Equal(t, int32(0), fnm.nodeSetNodeCountList.ByNodeSet()["nodeset-2"])

				fnm.AddNode()
				assert.Equal(t, int32(1), fnm.nodeSetNodeCountList.ByNodeSet()["nodeset-1"])
				assert.Equal(t, int32(1), fnm.nodeSetNodeCountList.ByNodeSet()["nodeset-2"])

				fnm.AddNode()
				assert.Equal(t, int32(2), fnm.nodeSetNodeCountList.ByNodeSet()["nodeset-1"])
				assert.Equal(t, int32(1), fnm.nodeSetNodeCountList.ByNodeSet()["nodeset-2"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fnm := NewFairNodesManager(logTest, tt.fields.nodeSetNodeCountList)
			tt.assertFunc(t, fnm)
		})
	}
}
