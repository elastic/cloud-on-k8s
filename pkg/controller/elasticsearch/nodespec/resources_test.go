// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"reflect"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
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
				{StatefulSet: sset.TestSset{Name: "master-only", Version: "7.2.0", Replicas: 3, Master: true, Data: false}.Build()},
				{StatefulSet: sset.TestSset{Name: "master-data", Version: "7.2.0", Replicas: 3, Master: true, Data: true}.Build()},
				{StatefulSet: sset.TestSset{Name: "data-only", Version: "7.2.0", Replicas: 3, Master: false, Data: true}.Build()},
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

func TestNodeSetListHasZoneAwareness(t *testing.T) {
	tests := []struct {
		name     string
		nodeSets []esv1.NodeSet
		want     bool
	}{
		{
			name:     "no node sets",
			nodeSets: nil,
			want:     false,
		},
		{
			name: "node sets without zone awareness",
			nodeSets: []esv1.NodeSet{
				{Name: "master"},
				{Name: "data"},
			},
			want: false,
		},
		{
			name: "some node sets with zone awareness",
			nodeSets: []esv1.NodeSet{
				{Name: "master"},
				{Name: "data", ZoneAwareness: &esv1.ZoneAwareness{}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := esv1.NodeSetList(tt.nodeSets).HasZoneAwareness()
			if got != tt.want {
				t.Errorf("NodeSetList.HasZoneAwareness() = %v, want %v", got, tt.want)
			}
		})
	}
}
