// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
)

func Test_validModeSpecificConfig(t *testing.T) {
	tests := []struct {
		name       string
		es         esv1.Elasticsearch
		wantErrors int
		wantMsgs   []string
	}{
		{
			name: "stateful: valid with no objectStore and no tier",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Mode:     esv1.ElasticsearchModeStateful,
					NodeSets: []esv1.NodeSet{{Name: "default", Count: 3}},
				},
			},
		},
		{
			name: "stateful: default mode with no objectStore and no tier",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{{Name: "default", Count: 3}},
				},
			},
		},
		{
			name: "stateful: objectStore is forbidden",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Mode:        esv1.ElasticsearchModeStateful,
					ObjectStore: &esv1.ObjectStoreConfig{Type: esv1.ObjectStoreTypeS3, Bucket: "b"},
					NodeSets:    []esv1.NodeSet{{Name: "default", Count: 3}},
				},
			},
			wantErrors: 1,
			wantMsgs:   []string{objectStoreForbiddenMsg},
		},
		{
			name: "stateful: tier is forbidden",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "9.4.0",
					Mode:    esv1.ElasticsearchModeStateful,
					NodeSets: []esv1.NodeSet{
						{Name: "default", Count: 3, Tier: esv1.IndexTier},
					},
				},
			},
			wantErrors: 1,
			wantMsgs:   []string{tierForbiddenMsg},
		},
		{
			name: "stateful: multiple tiers forbidden",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "9.4.0",
					Mode:    esv1.ElasticsearchModeStateful,
					NodeSets: []esv1.NodeSet{
						{Name: "a", Count: 1, Tier: esv1.IndexTier},
						{Name: "b", Count: 1, Tier: esv1.SearchTier},
					},
				},
			},
			wantErrors: 2,
			wantMsgs:   []string{tierForbiddenMsg},
		},
		{
			name: "stateful: index role is forbidden",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "9.4.0",
					Mode:    esv1.ElasticsearchModeStateful,
					NodeSets: []esv1.NodeSet{
						{Name: "default", Count: 3, Config: &commonv1.Config{Data: map[string]any{
							"node.roles": []string{"index"},
						}}},
					},
				},
			},
			wantErrors: 1,
			wantMsgs:   []string{statelessRoleInStatefulMsg},
		},
		{
			name: "stateful: search role is forbidden",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "9.4.0",
					Mode:    esv1.ElasticsearchModeStateful,
					NodeSets: []esv1.NodeSet{
						{Name: "default", Count: 3, Config: &commonv1.Config{Data: map[string]any{
							"node.roles": []string{"search"},
						}}},
					},
				},
			},
			wantErrors: 1,
			wantMsgs:   []string{statelessRoleInStatefulMsg},
		},
		{
			name: "stateless: version too old",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:     "9.3.0",
					Mode:        esv1.ElasticsearchModeStateless,
					ObjectStore: &esv1.ObjectStoreConfig{Type: esv1.ObjectStoreTypeS3, Bucket: "b"},
					NodeSets: []esv1.NodeSet{
						{Name: "index", Count: 1},
						{Name: "search", Count: 2},
					},
				},
			},
			wantErrors: 1,
			wantMsgs:   []string{statelessMinVersionMsg},
		},
		{
			name: "stateless: valid minimal config",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:     "9.4.0",
					Mode:        esv1.ElasticsearchModeStateless,
					ObjectStore: &esv1.ObjectStoreConfig{Type: esv1.ObjectStoreTypeS3, Bucket: "b"},
					NodeSets: []esv1.NodeSet{
						{Name: "index", Count: 1},
						{Name: "search", Count: 2},
					},
				},
			},
		},
		{
			name: "stateless: valid with explicit tiers",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:     "9.4.0",
					Mode:        esv1.ElasticsearchModeStateless,
					ObjectStore: &esv1.ObjectStoreConfig{Type: esv1.ObjectStoreTypeGCS, Bucket: "b"},
					NodeSets: []esv1.NodeSet{
						{Name: "my-indexers", Count: 1, Tier: esv1.IndexTier},
						{Name: "my-searchers", Count: 2, Tier: esv1.SearchTier},
					},
				},
			},
		},
		{
			name: "stateless: objectStore required",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "9.4.0",
					Mode:    esv1.ElasticsearchModeStateless,
					NodeSets: []esv1.NodeSet{
						{Name: "index", Count: 1},
						{Name: "search", Count: 2},
					},
				},
			},
			wantErrors: 1,
			wantMsgs:   []string{objectStoreRequiredMsg},
		},
		{
			name: "stateless: missing index tier",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:     "9.4.0",
					Mode:        esv1.ElasticsearchModeStateless,
					ObjectStore: &esv1.ObjectStoreConfig{Type: esv1.ObjectStoreTypeS3, Bucket: "b"},
					NodeSets: []esv1.NodeSet{
						{Name: "search", Count: 2},
					},
				},
			},
			wantErrors: 1,
			wantMsgs:   []string{tierIndexRequiredMsg},
		},
		{
			name: "stateless: missing search tier",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:     "9.4.0",
					Mode:        esv1.ElasticsearchModeStateless,
					ObjectStore: &esv1.ObjectStoreConfig{Type: esv1.ObjectStoreTypeS3, Bucket: "b"},
					NodeSets: []esv1.NodeSet{
						{Name: "index", Count: 1},
					},
				},
			},
			wantErrors: 1,
			wantMsgs:   []string{tierSearchRequiredMsg},
		},
		{
			name: "stateless: unresolvable tier name",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:     "9.4.0",
					Mode:        esv1.ElasticsearchModeStateless,
					ObjectStore: &esv1.ObjectStoreConfig{Type: esv1.ObjectStoreTypeS3, Bucket: "b"},
					NodeSets: []esv1.NodeSet{
						{Name: "index", Count: 1},
						{Name: "search", Count: 2},
						{Name: "data-nodes", Count: 3},
					},
				},
			},
			wantErrors: 1,
			wantMsgs:   []string{tierResolutionErrMsg},
		},
		{
			name: "stateless: all errors accumulate",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "9.4.0",
					Mode:    esv1.ElasticsearchModeStateless,
					NodeSets: []esv1.NodeSet{
						{Name: "data-nodes", Count: 1},
					},
				},
			},
			// objectStore missing + unresolvable tier + no index + no search
			wantErrors: 4,
		},
		{
			name: "stateless: index and search roles on same NodeSet rejected",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:     "9.4.0",
					Mode:        esv1.ElasticsearchModeStateless,
					ObjectStore: &esv1.ObjectStoreConfig{Type: esv1.ObjectStoreTypeS3, Bucket: "b"},
					NodeSets: []esv1.NodeSet{
						{Name: "index", Count: 1, Config: &commonv1.Config{Data: map[string]any{
							"node.roles": []string{"index", "search"},
						}}},
						{Name: "search", Count: 2},
					},
				},
			},
			wantErrors: 1,
			wantMsgs:   []string{indexAndSearchRolesConflictMsg},
		},
		{
			name: "stateless: single role is fine",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version:     "9.4.0",
					Mode:        esv1.ElasticsearchModeStateless,
					ObjectStore: &esv1.ObjectStoreConfig{Type: esv1.ObjectStoreTypeS3, Bucket: "b"},
					NodeSets: []esv1.NodeSet{
						{Name: "index", Count: 1, Config: &commonv1.Config{Data: map[string]any{
							"node.roles": []string{"index"},
						}}},
						{Name: "search", Count: 2},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validModeSpecificConfig(tt.es)
			assert.Len(t, errs, tt.wantErrors)
			for _, msg := range tt.wantMsgs {
				found := false
				for _, err := range errs {
					if strings.Contains(err.Detail, msg) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected error containing %q", msg)
			}
		})
	}
}

func Test_noModeChange(t *testing.T) {
	tests := []struct {
		name     string
		current  esv1.Elasticsearch
		proposed esv1.Elasticsearch
		wantErr  bool
	}{
		{
			name:     "same mode stateful",
			current:  esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Mode: esv1.ElasticsearchModeStateful}},
			proposed: esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Mode: esv1.ElasticsearchModeStateful}},
		},
		{
			name:     "same mode stateless",
			current:  esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Mode: esv1.ElasticsearchModeStateless}},
			proposed: esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Mode: esv1.ElasticsearchModeStateless}},
		},
		{
			name:     "same mode both empty",
			current:  esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{}},
			proposed: esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{}},
		},
		{
			name:     "stateful to stateless forbidden",
			current:  esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Mode: esv1.ElasticsearchModeStateful}},
			proposed: esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Mode: esv1.ElasticsearchModeStateless}},
			wantErr:  true,
		},
		{
			name:     "stateless to stateful forbidden",
			current:  esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Mode: esv1.ElasticsearchModeStateless}},
			proposed: esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Mode: esv1.ElasticsearchModeStateful}},
			wantErr:  true,
		},
		{
			name:     "empty to stateful allowed (equivalent)",
			current:  esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{}},
			proposed: esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Mode: esv1.ElasticsearchModeStateful}},
		},
		{
			name:     "stateful to empty allowed (equivalent)",
			current:  esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Mode: esv1.ElasticsearchModeStateful}},
			proposed: esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{}},
		},
		{
			name:     "empty to stateless forbidden",
			current:  esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{}},
			proposed: esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Mode: esv1.ElasticsearchModeStateless}},
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := noModeChange(tt.current, tt.proposed)
			if tt.wantErr {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func Test_statelessNodeRolesWarning(t *testing.T) {
	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		wantWarnings int
	}{
		{
			name: "stateful with node.roles: no warning",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{Name: "default", Count: 3, Config: &commonv1.Config{Data: map[string]any{
							"node.roles": []string{"master", "data"},
						}}},
					},
				},
			},
		},
		{
			name: "stateless without node.roles: no warning",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Mode: esv1.ElasticsearchModeStateless,
					NodeSets: []esv1.NodeSet{
						{Name: "index", Count: 1},
						{Name: "search", Count: 2},
					},
				},
			},
		},
		{
			name: "stateless with node.roles: warning",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Mode: esv1.ElasticsearchModeStateless,
					NodeSets: []esv1.NodeSet{
						{Name: "index", Count: 1, Config: &commonv1.Config{Data: map[string]any{
							"node.roles": []string{"index"},
						}}},
						{Name: "search", Count: 2},
					},
				},
			},
			wantWarnings: 1,
		},
		{
			name: "stateless with node.roles on multiple nodeSets: multiple warnings",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Mode: esv1.ElasticsearchModeStateless,
					NodeSets: []esv1.NodeSet{
						{Name: "index", Count: 1, Config: &commonv1.Config{Data: map[string]any{
							"node.roles": []string{"index"},
						}}},
						{Name: "search", Count: 2, Config: &commonv1.Config{Data: map[string]any{
							"node.roles": []string{"search"},
						}}},
					},
				},
			},
			wantWarnings: 2,
		},
		{
			name: "stateless with other config but no node.roles: no warning",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Mode: esv1.ElasticsearchModeStateless,
					NodeSets: []esv1.NodeSet{
						{Name: "index", Count: 1, Config: &commonv1.Config{Data: map[string]any{
							"node.store.allow_mmap": false,
						}}},
						{Name: "search", Count: 2},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := statelessNodeRolesWarning(tt.es)
			assert.Len(t, warnings, tt.wantWarnings)
		})
	}
}
