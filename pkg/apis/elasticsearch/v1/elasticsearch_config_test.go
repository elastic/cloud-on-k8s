// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/compare"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/pointer"
)

func TestConfig_RoleDefaults(t *testing.T) {
	type args struct {
		c   commonv1.Config
		ver version.Version
	}
	tests := []struct {
		name    string
		args    args
		wantCfg Node
	}{
		{
			name: "empty equals defaults",
			args: args{},
			wantCfg: Node{
				Master:    pointer.BoolPtr(true),
				Data:      pointer.BoolPtr(true),
				Ingest:    pointer.BoolPtr(true),
				ML:        pointer.BoolPtr(true),
				Transform: pointer.BoolPtr(false),
			},
		},
		{
			name: "set node.master=true",
			args: args{
				c: commonv1.Config{
					Data: map[string]interface{}{
						NodeMaster: true,
					},
				},
			},
			wantCfg: Node{
				Master:    pointer.BoolPtr(true),
				Data:      pointer.BoolPtr(true),
				Ingest:    pointer.BoolPtr(true),
				ML:        pointer.BoolPtr(true),
				Transform: pointer.BoolPtr(false),
			},
		},
		{
			name: "set node.data=false",
			args: args{
				c: commonv1.Config{
					Data: map[string]interface{}{
						NodeData: false,
					},
				},
			},
			wantCfg: Node{
				Master:    pointer.BoolPtr(true),
				Data:      pointer.BoolPtr(false),
				Ingest:    pointer.BoolPtr(true),
				ML:        pointer.BoolPtr(true),
				Transform: pointer.BoolPtr(false),
			},
		},
		{
			name: "defaults for versions above 7.7.0",
			args: args{
				ver: version.From(7, 7, 0),
			},
			wantCfg: Node{
				Master: pointer.BoolPtr(true),
				Data:   pointer.BoolPtr(true),
				Ingest: pointer.BoolPtr(true),
				ML:     pointer.BoolPtr(true),
			},
		},
		{
			name: "set node.data=false on 7.7.0",
			args: args{
				c: commonv1.Config{
					Data: map[string]interface{}{
						"node": map[string]interface{}{
							"data": false,
						},
					},
				},
				ver: version.From(7, 7, 0),
			},
			wantCfg: Node{
				Master: pointer.BoolPtr(true),
				Data:   pointer.BoolPtr(false),
				Ingest: pointer.BoolPtr(true),
				ML:     pointer.BoolPtr(true),
			},
		},
		{
			name: "set node.transform=true and node.data=false on 7.7.0",
			args: args{
				c: commonv1.Config{
					Data: map[string]interface{}{
						NodeData:      false,
						NodeTransform: true,
					},
				},
				ver: version.From(7, 7, 0),
			},
			wantCfg: Node{
				Master:    pointer.BoolPtr(true),
				Data:      pointer.BoolPtr(false),
				Ingest:    pointer.BoolPtr(true),
				ML:        pointer.BoolPtr(true),
				Transform: pointer.BoolPtr(true),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultCfg(tt.args.ver)
			err := UnpackConfig(&tt.args.c, tt.args.ver, &got)
			require.NoError(t, err)
			compare.JSONEqual(t, tt.wantCfg, got.Node)
		})
	}
}

var testFixture = commonv1.Config{
	Data: map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": 1.0,
			},
			"d": 1,
		},
		"a.b.foo": "bar",
		"e":       []interface{}{1, 2, 3},
		"f":       true,
	},
}

var expectedJSONized = commonv1.Config{
	Data: map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": 1.0,
			},
			"d": float64(1),
		},
		"a.b.foo": "bar",
		"e":       []interface{}{float64(1), float64(2), float64(3)},
		"f":       true,
	},
}

func TestConfig_HasRole(t *testing.T) {
	testCases := []struct {
		name           string
		node           *Node
		wantMaster     bool
		wantData       bool
		wantIngest     bool
		wantML         bool
		wantTransform  bool
		wantVotingOnly bool
	}{
		{
			name:           "nil node",
			wantMaster:     true,
			wantData:       true,
			wantIngest:     true,
			wantML:         true,
			wantTransform:  true,
			wantVotingOnly: false,
		},
		{
			name:           "empty node",
			node:           &Node{},
			wantMaster:     true,
			wantData:       true,
			wantIngest:     true,
			wantML:         true,
			wantTransform:  true,
			wantVotingOnly: false,
		},
		{
			name: "node role attributes (all)",
			node: &Node{
				Master:    pointer.BoolPtr(true),
				Data:      pointer.BoolPtr(true),
				Ingest:    pointer.BoolPtr(true),
				ML:        pointer.BoolPtr(true),
				Transform: pointer.BoolPtr(true),
			},
			wantMaster:     true,
			wantData:       true,
			wantIngest:     true,
			wantML:         true,
			wantTransform:  true,
			wantVotingOnly: false,
		},
		{
			name: "node role attributes (no data)",
			node: &Node{
				Data: pointer.BoolPtr(false),
			},
			wantMaster:     true,
			wantData:       false,
			wantIngest:     true,
			wantML:         true,
			wantTransform:  false,
			wantVotingOnly: false,
		},
		{
			name: "node role attributes (ingest only)",
			node: &Node{
				Master:     pointer.BoolPtr(false),
				Data:       pointer.BoolPtr(false),
				Ingest:     pointer.BoolPtr(true),
				ML:         pointer.BoolPtr(false),
				Transform:  pointer.BoolPtr(false),
				VotingOnly: pointer.BoolPtr(false),
			},
			wantMaster:     false,
			wantData:       false,
			wantIngest:     true,
			wantML:         false,
			wantTransform:  false,
			wantVotingOnly: false,
		},
		{
			name: "mixed node.roles and node role attributes",
			node: &Node{
				Master:     pointer.BoolPtr(false),
				Data:       pointer.BoolPtr(false),
				Ingest:     pointer.BoolPtr(true),
				ML:         pointer.BoolPtr(false),
				Transform:  pointer.BoolPtr(false),
				VotingOnly: pointer.BoolPtr(false),
				Roles:      []string{"master"},
			},
			wantMaster:     true,
			wantData:       false,
			wantIngest:     false,
			wantML:         false,
			wantTransform:  false,
			wantVotingOnly: false,
		},
		{
			name:           "node.roles (all)",
			node:           &Node{Roles: []string{"master", "data", "ingest", "ml", "transform"}},
			wantMaster:     true,
			wantData:       true,
			wantIngest:     true,
			wantML:         true,
			wantTransform:  true,
			wantVotingOnly: false,
		},
		{
			name:           "node.roles (master and data)",
			node:           &Node{Roles: []string{"master", "data"}},
			wantMaster:     true,
			wantData:       true,
			wantIngest:     false,
			wantML:         false,
			wantTransform:  false,
			wantVotingOnly: false,
		},
		{
			name:           "node.roles (ingest only)",
			node:           &Node{Roles: []string{"ingest"}},
			wantMaster:     false,
			wantData:       false,
			wantIngest:     true,
			wantML:         false,
			wantTransform:  false,
			wantVotingOnly: false,
		},
		{
			name:           "node.roles (no roles)",
			node:           &Node{Roles: []string{}},
			wantMaster:     false,
			wantData:       false,
			wantIngest:     false,
			wantML:         false,
			wantTransform:  false,
			wantVotingOnly: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantMaster, tc.node.HasMasterRole())
			require.Equal(t, tc.wantData, tc.node.HasDataRole())
			require.Equal(t, tc.wantIngest, tc.node.HasIngestRole())
			require.Equal(t, tc.wantML, tc.node.HasMLRole())
			require.Equal(t, tc.wantTransform, tc.node.HasTransformRole())
			require.Equal(t, tc.wantVotingOnly, tc.node.HasVotingOnlyRole())
		})
	}
}

func TestConfig_DeepCopyInto(t *testing.T) {
	tests := []struct {
		name     string
		in       commonv1.Config
		expected commonv1.Config
	}{
		{
			name:     "deep copy via JSON roundtrip changes some types",
			in:       testFixture,
			expected: expectedJSONized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out commonv1.Config
			tt.in.DeepCopyInto(&out)
			if diff := deep.Equal(out, tt.expected); diff != nil {
				t.Error(diff)
			}
		})
	}
}

func TestConfig_DeepCopy(t *testing.T) {
	tests := []struct {
		name string
		in   commonv1.Config
		want commonv1.Config
	}{
		{
			name: "deep copy via JSON roundtrip changes some types",
			in:   testFixture,
			want: expectedJSONized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := deep.Equal(tt.in.DeepCopy(), &tt.want); diff != nil {
				t.Error(diff)
			}
		})
	}
}

func TestConfig_Unpack(t *testing.T) {
	ver := version.From(7, 7, 0)
	tests := []struct {
		name    string
		args    *commonv1.Config
		want    ElasticsearchSettings
		wantErr bool
	}{
		{
			name: "no node config",
			args: &commonv1.Config{
				Data: map[string]interface{}{
					"cluster": map[string]interface{}{
						"initial_master_nodes": []string{"a", "b"},
					},
				},
			},
			want: ElasticsearchSettings{
				Cluster: ClusterSettings{
					InitialMasterNodes: []string{"a", "b"},
				},
			},
			wantErr: false,
		},
		{
			name: "happy path",
			args: &commonv1.Config{
				Data: map[string]interface{}{
					"node": map[string]interface{}{
						"master": false,
						"data":   true,
					},
					"cluster": map[string]interface{}{
						"initial_master_nodes": []string{"a", "b"},
					},
				},
			},
			want: ElasticsearchSettings{
				Node: &Node{
					Master: pointer.BoolPtr(false),
					Data:   pointer.BoolPtr(true),
				},
				Cluster: ClusterSettings{
					InitialMasterNodes: []string{"a", "b"},
				},
			},
			wantErr: false,
		},
		{
			name: "happy path with node roles",
			args: &commonv1.Config{
				Data: map[string]interface{}{
					"node": map[string]interface{}{
						"roles": []string{"master", "data"},
					},
					"cluster": map[string]interface{}{
						"initial_master_nodes": []string{"a", "b"},
					},
				},
			},
			want: ElasticsearchSettings{
				Node: &Node{
					Roles: []string{"master", "data"},
				},
				Cluster: ClusterSettings{
					InitialMasterNodes: []string{"a", "b"},
				},
			},
			wantErr: false,
		},
		{
			name:    "Unpack is nil safe",
			args:    nil,
			want:    ElasticsearchSettings{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ElasticsearchSettings{}
			err := UnpackConfig(tt.args, ver, &got)
			if tt.wantErr {
				require.Error(t, err)
			}

			require.Equal(t, tt.want, got)
		})
	}
}
