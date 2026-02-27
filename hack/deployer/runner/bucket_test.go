// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_newBucketConfig_S3Validation(t *testing.T) {
	basePlan := Plan{
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
	ctx := map[string]any{"ClusterName": "my-cluster"}

	tests := []struct {
		name    string
		s3      *S3BucketSettings
		wantErr string
	}{
		{
			name: "valid S3 settings",
			s3: &S3BucketSettings{
				IamUserPath:      "/deployer/",
				ManagedPolicyARN: "arn:aws:iam::123456789012:policy/my-policy",
			},
		},
		{
			name: "missing iamUserPath",
			s3: &S3BucketSettings{
				ManagedPolicyARN: "arn:aws:iam::123456789012:policy/my-policy",
			},
			wantErr: "bucket.s3.iamUserPath must not be empty",
		},
		{
			name: "missing managedPolicyARN",
			s3: &S3BucketSettings{
				IamUserPath: "/deployer/",
			},
			wantErr: "bucket.s3.managedPolicyARN must not be empty",
		},
		{
			name:    "both fields empty",
			s3:      &S3BucketSettings{},
			wantErr: "bucket.s3.iamUserPath must not be empty",
		},
		{
			name: "nil S3 settings is valid (non-EKS provider)",
			s3:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := basePlan
			plan.Bucket.S3 = tt.s3

			cfg, err := newBucketConfig(plan, ctx, "eu-west-2")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "my-cluster-dev", cfg.Name)
			assert.Equal(t, "eu-west-2", cfg.Region)
			if tt.s3 != nil {
				assert.Equal(t, tt.s3.IamUserPath, cfg.S3.IAMUserPath)
				assert.Equal(t, tt.s3.ManagedPolicyARN, cfg.S3.ManagedPolicyARN)
			}
		})
	}
}
