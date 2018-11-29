package support

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/assert"
	"testing"
)

func createTopology(count int, nodeTypes v1alpha1.NodeTypesSpec) v1alpha1.ElasticsearchTopologySpec {
	return v1alpha1.ElasticsearchTopologySpec{
		NodeCount: int32(count),
		NodeTypes: nodeTypes,
	}
}

func TopologiesWith(nMasters, nData, nMasterData int) []v1alpha1.ElasticsearchTopologySpec {
	topologies := []v1alpha1.ElasticsearchTopologySpec{}
	topologies = append(topologies, createTopology(nMasters, v1alpha1.NodeTypesSpec{
		Master: true,
	}))
	topologies = append(topologies, createTopology(nData, v1alpha1.NodeTypesSpec{
		Data: true,
	}))
	topologies = append(topologies, createTopology(nMasterData, v1alpha1.NodeTypesSpec{
		Master: true,
		Data:   true,
	}))
	return topologies
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
			topologies := TopologiesWith(tt.args.nMasters, tt.args.nData, tt.args.nMasterData)
			mmn := ComputeMinimumMasterNodes(topologies)
			assert.Equal(t, tt.want, mmn, "Unmatching minimum master nodes")
		})
	}
}
