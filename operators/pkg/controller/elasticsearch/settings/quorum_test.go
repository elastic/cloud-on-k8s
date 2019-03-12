// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func createTopologyElement(count int, nodeTypes v1alpha1.NodeTypesSpec) v1alpha1.TopologyElementSpec {
	return v1alpha1.TopologyElementSpec{
		NodeCount: int32(count),
		NodeTypes: nodeTypes,
	}
}

func TopologyWith(nMasters, nData, nMasterData int) []v1alpha1.TopologyElementSpec {
	var topology []v1alpha1.TopologyElementSpec
	topology = append(topology, createTopologyElement(nMasters, v1alpha1.NodeTypesSpec{
		Master: true,
	}))
	topology = append(topology, createTopologyElement(nData, v1alpha1.NodeTypesSpec{
		Data: true,
	}))
	topology = append(topology, createTopologyElement(nMasterData, v1alpha1.NodeTypesSpec{
		Master: true,
		Data:   true,
	}))
	return topology
}

func TestComputeMinimumMasterNodes(t *testing.T) {
	type args struct {
		nMasters    int
		nData       int
		nMasterData int
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{args: args{nMasters: 0}, want: 0},
		{args: args{nMasters: 1}, want: 1},
		{args: args{nMasters: 1, nData: 1}, want: 1},
		{args: args{nMasters: 1, nData: 10}, want: 1},
		{args: args{nMasters: 2}, want: 2},
		{args: args{nMasters: 2, nData: 10}, want: 2},
		{args: args{nMasters: 3}, want: 2},
		{args: args{nMasters: 4}, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topology := TopologyWith(tt.args.nMasters, tt.args.nData, tt.args.nMasterData)
			mmn := ComputeMinimumMasterNodes(topology)
			assert.Equal(t, tt.want, mmn, "Unmatching minimum master nodes")
		})
	}
}
