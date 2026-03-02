// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/runner/bucket"
)

func Test_newBucketConfig(t *testing.T) {
	ctx := map[string]any{"ClusterName": "my-cluster"}

	plan := Plan{
		Id:          "test",
		ClusterName: "my-cluster",
		Bucket: &BucketSettings{
			Name:         "{{ .ClusterName }}-dev",
			StorageClass: "STANDARD",
			Secret: BucketSecretSettings{
				Name:      "my-secret",
				Namespace: "default",
			},
		},
	}

	cfg, err := newBucketConfig(plan, ctx, "eu-west-2")
	require.NoError(t, err)
	assert.Equal(t, "my-cluster-dev", cfg.Name)
	assert.Equal(t, "eu-west-2", cfg.Region)
	assert.Equal(t, "my-secret", cfg.SecretName)
	assert.Equal(t, "default", cfg.SecretNamespace)
	assert.Equal(t, "my-cluster", cfg.Labels["cluster_name"])
	assert.Equal(t, "test", cfg.Labels["plan_id"])
	assert.Equal(t, "eck-deployer", cfg.Labels["managed_by"])
}

// fakeBucketManager records which methods were called.
type fakeBucketManager struct {
	created bool
	deleted bool
}

func (f *fakeBucketManager) Create() error { f.created = true; return nil }
func (f *fakeBucketManager) Delete() error { f.deleted = true; return nil }

func Test_createBucketIfConfigured(t *testing.T) {
	tests := []struct {
		name           string
		plan           Plan
		managerErr     error
		wantErr        string
		wantCreated    bool
		wantFactoryHit bool
	}{
		{
			name:           "nil bucket skips creation",
			plan:           Plan{},
			wantFactoryHit: false,
		},
		{
			name:           "calls Create on manager",
			plan:           Plan{Bucket: &BucketSettings{}},
			wantCreated:    true,
			wantFactoryHit: true,
		},
		{
			name:           "propagates manager construction error",
			plan:           Plan{Bucket: &BucketSettings{}},
			managerErr:     errors.New("boom"),
			wantErr:        "boom",
			wantFactoryHit: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := &fakeBucketManager{}
			factoryHit := false
			err := createBucketIfConfigured(tt.plan, func() (bucket.Manager, error) {
				factoryHit = true
				if tt.managerErr != nil {
					return nil, tt.managerErr
				}
				return mgr, nil
			})
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantFactoryHit, factoryHit)
			assert.Equal(t, tt.wantCreated, mgr.created)
		})
	}
}

func Test_deleteBucketIfConfigured(t *testing.T) {
	tests := []struct {
		name           string
		plan           Plan
		managerErr     error
		wantErr        string
		wantDeleted    bool
		wantFactoryHit bool
	}{
		{
			name:           "nil bucket skips deletion",
			plan:           Plan{},
			wantFactoryHit: false,
		},
		{
			name:           "calls Delete on manager",
			plan:           Plan{Bucket: &BucketSettings{}},
			wantDeleted:    true,
			wantFactoryHit: true,
		},
		{
			name:           "propagates manager construction error",
			plan:           Plan{Bucket: &BucketSettings{}},
			managerErr:     errors.New("boom"),
			wantErr:        "boom",
			wantFactoryHit: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := &fakeBucketManager{}
			factoryHit := false
			err := deleteBucketIfConfigured(tt.plan, func() (bucket.Manager, error) {
				factoryHit = true
				if tt.managerErr != nil {
					return nil, tt.managerErr
				}
				return mgr, nil
			})
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantFactoryHit, factoryHit)
			assert.Equal(t, tt.wantDeleted, mgr.deleted)
		})
	}
}
