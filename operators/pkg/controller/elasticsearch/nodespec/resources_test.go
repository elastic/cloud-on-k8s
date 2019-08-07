// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	"reflect"
	"testing"
)

func TestResourcesList_MasterNodesNames(t *testing.T) {
	tests := []struct {
		name string
		l    ResourcesList
		want []string
	}{
		{
			name: "no nodes",
			l:    nil,
			want: nil,
		},
		{
			name: "3 master-only nodes, 3 master-data nodes, 3 data nodes",
			l: ResourcesList{
				{StatefulSet: CreateTestSset("master-only", "7.2.0", 3, true, false)},
				{StatefulSet: CreateTestSset("master-data", "7.2.0", 3, true, true)},
				{StatefulSet: CreateTestSset("data-only", "7.2.0", 3, false, true)},
			},
			want: []string{
				"master-only-0", "master-only-1", "master-only-2",
				"master-data-0", "master-data-1", "master-data-2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.l.MasterNodesNames(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ResourcesList.MasterNodesNames() = %v, want %v", got, tt.want)
			}
		})
	}
}
