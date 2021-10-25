// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoscaler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/resources"
)

func Test_distributeFairly(t *testing.T) {
	type args struct {
		nodeSets          resources.NodeSetNodeCountList
		expectedNodeCount int32
	}
	tests := []struct {
		name             string
		args             args
		expectedNodeSets resources.NodeSetNodeCountList
	}{
		{
			name: "nodeSet is nil, no panic",
			args: args{
				nodeSets:          nil,
				expectedNodeCount: 2,
			},
			expectedNodeSets: nil,
		},
		{
			name: "nodeSet is empty, no panic",
			args: args{
				nodeSets:          []resources.NodeSetNodeCount{},
				expectedNodeCount: 2,
			},
			expectedNodeSets: []resources.NodeSetNodeCount{},
		},
		{
			name: "One nodeSet",
			args: args{
				nodeSets:          []resources.NodeSetNodeCount{{Name: "nodeset-1"}},
				expectedNodeCount: 2,
			},
			expectedNodeSets: []resources.NodeSetNodeCount{{Name: "nodeset-1", NodeCount: 2}},
		},
		{
			name: "Two nodeSet",
			args: args{
				nodeSets:          []resources.NodeSetNodeCount{{Name: "nodeset-1"}, {Name: "nodeset-2"}},
				expectedNodeCount: 3,
			},
			expectedNodeSets: []resources.NodeSetNodeCount{{Name: "nodeset-1", NodeCount: 2}, {Name: "nodeset-2", NodeCount: 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			distributeFairly(tt.args.nodeSets, tt.args.expectedNodeCount)
			assert.ElementsMatch(t, tt.args.nodeSets, tt.expectedNodeSets)
		})
	}
}
