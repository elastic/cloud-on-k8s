// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
)

// Test_updateState tests updateState and consequently mutateSnapshotRepositorySettings.
func Test_updateState(t *testing.T) {
	esSample := types.NamespacedName{
		Namespace: "esNs",
		Name:      "esName",
	}

	clusterSettings := &commonv1.Config{Data: map[string]any{
		"indices.recovery.max_bytes_per_sec": "100mb",
	}}
	snapshotLifecyclePolicies := &commonv1.Config{Data: map[string]any{
		"test-snapshots": map[string]any{
			"schedule":   "0 1 2 3 4 ?",
			"name":       "<production-snap-{now/d}>",
			"repository": "es-snapshots",
			"config": map[string]any{
				"indices":              []any{"*"},
				"ignore_unavailable":   true,
				"include_global_state": false,
			},
			"retention": map[string]any{
				"expire_after": "7d",
				"min_count":    "1",
				"max_count":    "20",
			},
		},
	}}
	roleMappings := &commonv1.Config{Data: map[string]any{
		"test-role-mapping": map[string]any{
			"enabled": true,
			"metadata": map[string]any{
				"_foo": "something_else",
				"uuid": "b9a59ba9-6b92-4be3-bb8d-02bb270cb3a7",
			},
			"roles": []any{"fleet_user"},
			"rules": map[string]any{
				"field": map[string]any{
					"username": "*",
				},
			},
		},
	}}
	indexLifecyclePolicies := &commonv1.Config{Data: map[string]any{
		"test-policy": map[string]any{
			"phases": map[string]any{
				"delete": map[string]any{
					"actions": map[string]any{
						"delete": map[string]any{},
					},
					"min_age": "30d",
				},
				"warm": map[string]any{
					"actions": map[string]any{
						"forcemerge": map[string]any{
							"max_num_segments": float64(1),
						},
					},
					"min_age": "10d",
				},
			},
		},
	}}
	ingestPipelines := &commonv1.Config{Data: map[string]any{
		"test-ingest-pipeline": map[string]any{
			"processors": []any{map[string]any{
				"set": map[string]any{
					"field": "my-keyword-field",
					"value": "foo",
				},
			}},
		},
	}}
	componentTemplates := &commonv1.Config{Data: map[string]any{
		"test-component-template": map[string]any{
			"template": map[string]any{
				"mappings": map[string]any{
					"properties": map[string]any{
						"@timestamp": map[string]any{
							"type": "date",
						},
					},
				},
			},
		},
	}}
	composableIndexTemplates := &commonv1.Config{Data: map[string]any{
		"test-template": map[string]any{
			"composed_of":    []any{"test-component-template"},
			"index_patterns": []any{"te*", "bar*"},
			"priority":       float64(500),
			"template": map[string]any{
				"aliases": map[string]any{
					"mydata": map[string]any{},
				},
				"mappings": map[string]any{
					"_source": map[string]any{
						"enabled": true,
					},
					"properties": map[string]any{
						"created_at": map[string]any{
							"format": "EEE MMM dd HH:mm:ss Z yyyy",
							"type":   "date",
						},
					},
				},
				"settings": map[string]any{
					"number_of_shards": float64(1),
				},
			},
			"version": float64(1),
		},
	}}

	type args struct {
		policy policyv1alpha1.StackConfigPolicy
	}
	tests := []struct {
		name    string
		args    args
		want    SettingsState
		wantErr error
	}{
		{
			name: "empty settings",
			args: args{policy: policyv1alpha1.StackConfigPolicy{}},
			want: newEmptySettingsState(),
		},
		{
			name: "gcs, azure and s3 snapshot repository settings: default base_path",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]any{
					"repo-gcs": map[string]any{
						"type": "gcs",
						"settings": map[string]any{
							"bucket": "bucket",
						},
					},
					"repo-azure": map[string]any{
						"type": "azure",
						"settings": map[string]any{
							"bucket": "bucket",
						},
					},
					"repo-s3": map[string]any{
						"type": "s3",
						"settings": map[string]any{
							"bucket": "bucket",
						},
					},
				}},
			}}}},
			want: SettingsState{
				ClusterSettings: &commonv1.Config{Data: map[string]any{}},
				SnapshotRepositories: &commonv1.Config{Data: map[string]any{
					"repo-gcs": map[string]any{
						"type": "gcs",
						"settings": map[string]any{
							"bucket":    "bucket",
							"base_path": "snapshots/esNs-esName",
						},
					},
					"repo-azure": map[string]any{
						"type": "azure",
						"settings": map[string]any{
							"bucket":    "bucket",
							"base_path": "snapshots/esNs-esName",
						},
					},
					"repo-s3": map[string]any{
						"type": "s3",
						"settings": map[string]any{
							"bucket":    "bucket",
							"base_path": "snapshots/esNs-esName",
						},
					},
				}},
				SLM:                    &commonv1.Config{Data: map[string]any{}},
				RoleMappings:           &commonv1.Config{Data: map[string]any{}},
				IndexLifecyclePolicies: &commonv1.Config{Data: map[string]any{}},
				IngestPipelines:        &commonv1.Config{Data: map[string]any{}},
				IndexTemplates: &IndexTemplates{
					ComponentTemplates:       &commonv1.Config{Data: map[string]any{}},
					ComposableIndexTemplates: &commonv1.Config{Data: map[string]any{}},
				},
			},
		},
		{
			name: "gcs, azure and s3 snapshot repository settings: set base_path",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]any{
					"repo-gcs": map[string]any{
						"type": "gcs",
						"settings": map[string]any{
							"bucket":    "bucket",
							"base_path": "snapshots/es",
						},
					},
					"repo-azure": map[string]any{
						"type": "azure",
						"settings": map[string]any{
							"bucket":    "bucket",
							"base_path": "es-snapshots",
						},
					},
					"repo-s3": map[string]any{
						"type": "s3",
						"settings": map[string]any{
							"bucket":    "bucket",
							"base_path": "a/b/c",
						},
					},
				}},
			}}}},
			want: SettingsState{
				ClusterSettings: &commonv1.Config{Data: map[string]any{}},
				SnapshotRepositories: &commonv1.Config{Data: map[string]any{
					"repo-gcs": map[string]any{
						"type": "gcs",
						"settings": map[string]any{
							"bucket":    "bucket",
							"base_path": "snapshots/es",
						},
					},
					"repo-azure": map[string]any{
						"type": "azure",
						"settings": map[string]any{
							"bucket":    "bucket",
							"base_path": "es-snapshots",
						},
					},
					"repo-s3": map[string]any{
						"type": "s3",
						"settings": map[string]any{
							"bucket":    "bucket",
							"base_path": "a/b/c",
						},
					},
				}},
				SLM:                    &commonv1.Config{Data: map[string]any{}},
				RoleMappings:           &commonv1.Config{Data: map[string]any{}},
				IndexLifecyclePolicies: &commonv1.Config{Data: map[string]any{}},
				IngestPipelines:        &commonv1.Config{Data: map[string]any{}},
				IndexTemplates: &IndexTemplates{
					ComponentTemplates:       &commonv1.Config{Data: map[string]any{}},
					ComposableIndexTemplates: &commonv1.Config{Data: map[string]any{}},
				},
			},
		},
		{
			name: "fs and hdfs snapshot repository: append cluster name to the location/path",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]any{
					"repo-fs": map[string]any{
						"type": "fs",
						"settings": map[string]any{
							"location": "/mnt/backup",
						},
					},
					"repo-hdfs": map[string]any{
						"type": "hdfs",
						"settings": map[string]any{
							"path": "/mnt/backup",
						},
					},
				}},
			}}}},
			want: SettingsState{
				ClusterSettings: &commonv1.Config{Data: map[string]any{}},
				SnapshotRepositories: &commonv1.Config{Data: map[string]any{
					"repo-fs": map[string]any{
						"type": "fs",
						"settings": map[string]any{
							"location": "/mnt/backup/esNs-esName",
						},
					},
					"repo-hdfs": map[string]any{
						"type": "hdfs",
						"settings": map[string]any{
							"path": "/mnt/backup/esNs-esName",
						},
					},
				}},
				SLM:                    &commonv1.Config{Data: map[string]any{}},
				RoleMappings:           &commonv1.Config{Data: map[string]any{}},
				IndexLifecyclePolicies: &commonv1.Config{Data: map[string]any{}},
				IngestPipelines:        &commonv1.Config{Data: map[string]any{}},
				IndexTemplates: &IndexTemplates{
					ComponentTemplates:       &commonv1.Config{Data: map[string]any{}},
					ComposableIndexTemplates: &commonv1.Config{Data: map[string]any{}},
				},
			},
		},
		{
			name: "source and url snapshot repository: no update",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]any{
					"repo-url": map[string]any{
						"type": "url",
						"settings": map[string]any{
							"url": "file:/mount/backups",
						},
					},
					"repo-source": map[string]any{
						"type": "source",
						"settings": map[string]any{
							"delegate_type": "source",
							"location":      "another_repository",
						},
					},
				}},
			}}}},
			want: SettingsState{
				ClusterSettings: &commonv1.Config{Data: map[string]any{}},
				SnapshotRepositories: &commonv1.Config{Data: map[string]any{
					"repo-url": map[string]any{
						"type": "url",
						"settings": map[string]any{
							"url": "file:/mount/backups",
						},
					},
					"repo-source": map[string]any{
						"type": "source",
						"settings": map[string]any{
							"delegate_type": "source",
							"location":      "another_repository",
						},
					},
				}},
				SLM:                    &commonv1.Config{Data: map[string]any{}},
				RoleMappings:           &commonv1.Config{Data: map[string]any{}},
				IndexLifecyclePolicies: &commonv1.Config{Data: map[string]any{}},
				IngestPipelines:        &commonv1.Config{Data: map[string]any{}},
				IndexTemplates: &IndexTemplates{
					ComponentTemplates:       &commonv1.Config{Data: map[string]any{}},
					ComposableIndexTemplates: &commonv1.Config{Data: map[string]any{}},
				},
			},
		},
		{
			name: "invalid type for snapshot repositories definition",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]any{
					"repo": "invalid-type",
				}},
			}}}},
			wantErr: errors.New(`invalid type (string) for definition of snapshot repository "repo" of Elasticsearch "esNs/esName"`),
		},
		{
			name: "invalid type for snapshot repositories settings",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]any{
					"repo": map[string]any{
						"settings": "invalid-type",
					},
				}},
			}}}},
			wantErr: errors.New("invalid type (string) for snapshot repository settings"),
		},
		{
			name: "invalid type for fs snapshot repository location",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]any{
					"repo": map[string]any{
						"type": "fs",
						"settings": map[string]any{
							"location": 42,
						},
					},
				}},
			}}}},
			wantErr: errors.New("invalid type (float64) for snapshot repository location"),
		},
		{
			name: "invalid type for hdfs snapshot repository path",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]any{
					"repo": map[string]any{
						"type": "hdfs",
						"settings": map[string]any{
							"path": 42,
						},
					},
				}},
			}}}},
			wantErr: errors.New("invalid type (float64) for snapshot repository path"),
		},
		{
			name: "other settings: no mutation",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				ClusterSettings:           clusterSettings,
				SnapshotRepositories:      &commonv1.Config{Data: map[string]any{}},
				SnapshotLifecyclePolicies: snapshotLifecyclePolicies,
				SecurityRoleMappings:      roleMappings,
				IndexLifecyclePolicies:    indexLifecyclePolicies,
				IngestPipelines:           ingestPipelines,
				IndexTemplates: policyv1alpha1.IndexTemplates{
					ComposableIndexTemplates: composableIndexTemplates,
					ComponentTemplates:       componentTemplates,
				},
			}}}},
			want: SettingsState{
				ClusterSettings:        clusterSettings,
				SnapshotRepositories:   &commonv1.Config{Data: map[string]any{}},
				SLM:                    snapshotLifecyclePolicies,
				RoleMappings:           roleMappings,
				IndexLifecyclePolicies: indexLifecyclePolicies,
				IngestPipelines:        ingestPipelines,
				IndexTemplates: &IndexTemplates{
					ComposableIndexTemplates: composableIndexTemplates,
					ComponentTemplates:       componentTemplates,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := Settings{}
			err := settings.updateState(esSample, tt.args.policy)
			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				return
			}
			assert.NoError(t, err)
			if !reflect.DeepEqual(settings.State, tt.want) {
				fmt.Println(cmp.Diff(settings.State, tt.want))
				t.Errorf("settings.updateState(policy, es) = %v, want %v", settings.State, tt.want)
			}
		})
	}
}
