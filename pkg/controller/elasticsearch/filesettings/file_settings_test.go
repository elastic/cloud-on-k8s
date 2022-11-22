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
			want: newSettingsState(),
		},
		{
			name: "cluster settings: no update",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				ClusterSettings: &commonv1.Config{Data: map[string]interface{}{
					"indices.recovery.max_bytes_per_sec": "100mb",
				}},
			}}}},
			want: SettingsState{
				ClusterSettings: &commonv1.Config{Data: map[string]interface{}{
					"indices.recovery.max_bytes_per_sec": "100mb",
				}},
				SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{}},
				SLM:                  &commonv1.Config{Data: map[string]interface{}{}},
			},
		},
		{
			name: "gcs, azure and s3 snapshot repository settings: adding a base_path",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{
					"repo-gcs": map[string]interface{}{
						"type": "gcs",
						"settings": map[string]interface{}{
							"bucket": "bucket",
						},
					},
					"repo-azure": map[string]interface{}{
						"type": "azure",
						"settings": map[string]interface{}{
							"bucket": "bucket",
						},
					},
					"repo-s3": map[string]interface{}{
						"type": "s3",
						"settings": map[string]interface{}{
							"bucket": "bucket",
						},
					},
				}},
			}}}},
			want: SettingsState{
				ClusterSettings: &commonv1.Config{Data: map[string]interface{}{}},
				SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{
					"repo-gcs": map[string]interface{}{
						"type": "gcs",
						"settings": map[string]interface{}{
							"bucket":    "bucket",
							"base_path": "snapshots/esNs-esName",
						},
					},
					"repo-azure": map[string]interface{}{
						"type": "azure",
						"settings": map[string]interface{}{
							"bucket":    "bucket",
							"base_path": "snapshots/esNs-esName",
						},
					},
					"repo-s3": map[string]interface{}{
						"type": "s3",
						"settings": map[string]interface{}{
							"bucket":    "bucket",
							"base_path": "snapshots/esNs-esName",
						},
					},
				}},
				SLM: &commonv1.Config{Data: map[string]interface{}{}},
			},
		},
		{
			name: "fs and hdfs snapshot repository: append cluster name to the location/path",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{
					"repo-fs": map[string]interface{}{
						"type": "fs",
						"settings": map[string]interface{}{
							"location": "/mnt/backup",
						},
					},
					"repo-hdfs": map[string]interface{}{
						"type": "hdfs",
						"settings": map[string]interface{}{
							"path": "/mnt/backup",
						},
					},
				}},
			}}}},
			want: SettingsState{
				ClusterSettings: &commonv1.Config{Data: map[string]interface{}{}},
				SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{
					"repo-fs": map[string]interface{}{
						"type": "fs",
						"settings": map[string]interface{}{
							"location": "/mnt/backup/esNs-esName",
						},
					},
					"repo-hdfs": map[string]interface{}{
						"type": "hdfs",
						"settings": map[string]interface{}{
							"path": "/mnt/backup/esNs-esName",
						},
					},
				}},
				SLM: &commonv1.Config{Data: map[string]interface{}{}},
			},
		},
		{
			name: "source and url snapshot repository: no update",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{
					"repo-url": map[string]interface{}{
						"type": "url",
						"settings": map[string]interface{}{
							"url": "file:/mount/backups",
						},
					},
					"repo-source": map[string]interface{}{
						"type": "source",
						"settings": map[string]interface{}{
							"delegate_type": "source",
							"location":      "another_repository",
						},
					},
				}},
			}}}},
			want: SettingsState{
				ClusterSettings: &commonv1.Config{Data: map[string]interface{}{}},
				SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{
					"repo-url": map[string]interface{}{
						"type": "url",
						"settings": map[string]interface{}{
							"url": "file:/mount/backups",
						},
					},
					"repo-source": map[string]interface{}{
						"type": "source",
						"settings": map[string]interface{}{
							"delegate_type": "source",
							"location":      "another_repository",
						},
					},
				}},
				SLM: &commonv1.Config{Data: map[string]interface{}{}},
			},
		},
		{
			name: "invalid type for snapshot repositories definition",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{
					"repo": "invalid-type",
				}},
			}}}},
			wantErr: errors.New(`invalid type (string) for definition of snapshot repository "repo" of Elasticsearch "esNs/esName"`),
		},
		{
			name: "invalid type for snapshot repositories settings",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{
					"repo": map[string]interface{}{
						"settings": "invalid-type",
					},
				}},
			}}}},
			wantErr: errors.New("invalid type (string) for snapshot repository settings"),
		},
		{
			name: "invalid type for fs snapshot repository location",
			args: args{policy: policyv1alpha1.StackConfigPolicy{Spec: policyv1alpha1.StackConfigPolicySpec{Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
				SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{
					"repo": map[string]interface{}{
						"type": "fs",
						"settings": map[string]interface{}{
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
				SnapshotRepositories: &commonv1.Config{Data: map[string]interface{}{
					"repo": map[string]interface{}{
						"type": "hdfs",
						"settings": map[string]interface{}{
							"path": 42,
						},
					},
				}},
			}}}},
			wantErr: errors.New("invalid type (float64) for snapshot repository path"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := NewEmptySettings(int64(1))
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
