// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestElasticsearch_IsStateless(t *testing.T) {
	tests := []struct {
		name string
		mode ElasticsearchMode
		want bool
	}{
		{
			name: "stateless mode",
			mode: ElasticsearchModeStateless,
			want: true,
		},
		{
			name: "stateful mode",
			mode: ElasticsearchModeStateful,
			want: false,
		},
		{
			name: "empty mode (default)",
			mode: "",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := &Elasticsearch{
				Spec: ElasticsearchSpec{
					Mode: tt.mode,
				},
			}
			assert.Equal(t, tt.want, es.IsStateless())
		})
	}
}

func TestNodeSet_ResolvedTier(t *testing.T) {
	tests := []struct {
		name     string
		nodeSet  NodeSet
		wantTier StatelessTier
		wantErr  bool
	}{
		{
			name:     "explicit tier takes precedence",
			nodeSet:  NodeSet{Name: "my-custom-name", Tier: SearchTier},
			wantTier: SearchTier,
			wantErr:  false,
		},
		{
			name:     "infer index from name prefix",
			nodeSet:  NodeSet{Name: "index"},
			wantTier: IndexTier,
			wantErr:  false,
		},
		{
			name:     "infer index from name with suffix",
			nodeSet:  NodeSet{Name: "index-az1"},
			wantTier: IndexTier,
			wantErr:  false,
		},
		{
			name:     "infer search from name prefix",
			nodeSet:  NodeSet{Name: "search"},
			wantTier: SearchTier,
			wantErr:  false,
		},
		{
			name:     "infer search from name with suffix",
			nodeSet:  NodeSet{Name: "search-hot"},
			wantTier: SearchTier,
			wantErr:  false,
		},
		{
			name:     "infer master from name prefix",
			nodeSet:  NodeSet{Name: "master"},
			wantTier: MasterTier,
			wantErr:  false,
		},
		{
			name:     "infer master from name with suffix",
			nodeSet:  NodeSet{Name: "master-az2"},
			wantTier: MasterTier,
			wantErr:  false,
		},
		{
			name:     "infer ml from name prefix",
			nodeSet:  NodeSet{Name: "ml"},
			wantTier: MLTier,
			wantErr:  false,
		},
		{
			name:     "infer ml from name with suffix",
			nodeSet:  NodeSet{Name: "ml-workers"},
			wantTier: MLTier,
			wantErr:  false,
		},
		{
			name:     "case insensitive inference",
			nodeSet:  NodeSet{Name: "Index-pool"},
			wantTier: IndexTier,
			wantErr:  false,
		},
		{
			name:    "unknown name without tier fails",
			nodeSet: NodeSet{Name: "data-nodes"},
			wantErr: true,
		},
		{
			name:     "explicit tier overrides conflicting name",
			nodeSet:  NodeSet{Name: "index-nodes", Tier: MasterTier},
			wantTier: MasterTier,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, err := tt.nodeSet.ResolvedTier()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTier, tier)
		})
	}
}
