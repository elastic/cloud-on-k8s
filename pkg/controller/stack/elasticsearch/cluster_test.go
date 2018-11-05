package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeMinimumMasterNodes(t *testing.T) {
	type args struct {
		nodeCount int
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{args: args{nodeCount: 1}, want: 1},
		{args: args{nodeCount: 2}, want: 2},
		{args: args{nodeCount: 3}, want: 2},
		{args: args{nodeCount: 4}, want: 3},
		{args: args{nodeCount: 5}, want: 3},
		{args: args{nodeCount: 6}, want: 4},
		{args: args{nodeCount: 100}, want: 51},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mmn := ComputeMinimumMasterNodes(tt.args.nodeCount)
			assert.Equal(t, tt.want, mmn, "Unmatching minimum master nodes")
		})
	}
}
