// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bucket

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_isNotFound(t *testing.T) {
	tests := []struct {
		name       string
		cmdOutput  string
		indicators []string
		want       bool
	}{
		{
			name:       "matches single indicator",
			cmdOutput:  "ERROR: (gcloud.storage.buckets.describe) NOT_FOUND: 404",
			indicators: []string{"NOT_FOUND"},
			want:       true,
		},
		{
			name:       "matches second indicator",
			cmdOutput:  "BucketNotFoundException: 404 gs://my-bucket",
			indicators: []string{"NOT_FOUND", "BucketNotFoundException"},
			want:       true,
		},
		{
			name:       "no match on permission error",
			cmdOutput:  "ERROR: permission denied for bucket my-bucket",
			indicators: []string{"NOT_FOUND", "BucketNotFoundException"},
			want:       false,
		},
		{
			name:       "empty output",
			cmdOutput:  "",
			indicators: []string{"NOT_FOUND"},
			want:       false,
		},
		{
			name:       "no indicators",
			cmdOutput:  "some output",
			indicators: nil,
			want:       false,
		},
		{
			name:       "case sensitive",
			cmdOutput:  "not_found",
			indicators: []string{"NOT_FOUND"},
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isNotFound(tt.cmdOutput, tt.indicators...))
		})
	}
}

func TestAzureManager_storageAccountName(t *testing.T) {
	tests := []struct {
		name       string
		bucketName string
	}{
		{name: "short name", bucketName: "my-bucket"},
		{name: "long name truncated", bucketName: "this-is-a-very-long-bucket-name-that-exceeds-limits"},
		{name: "with dots", bucketName: "my.bucket.name"},
		{name: "with underscores", bucketName: "my_bucket_name"},
		{name: "no collision: hyphen vs dot", bucketName: "my-bucket"},
		{name: "no collision: dot variant", bucketName: "my.bucket"},
	}

	seen := map[string]string{} // account name -> bucket name
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AzureManager{cfg: Config{Name: tt.bucketName}}
			got := a.storageAccountName()
			assert.LessOrEqual(t, len(got), 24, "must be at most 24 characters")
			assert.GreaterOrEqual(t, len(got), 3, "must be at least 3 characters")
			assert.Regexp(t, `^[a-z0-9]+$`, got, "must be lowercase alphanumeric only")
			assert.True(t, strings.HasPrefix(got, "eckbkt"), "must start with eckbkt prefix")

			if prev, exists := seen[got]; exists && prev != tt.bucketName {
				t.Errorf("collision: bucket names %q and %q both map to storage account %q", prev, tt.bucketName, got)
			}
			seen[got] = tt.bucketName
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid lowercase", input: "my-bucket-123"},
		{name: "valid with dots", input: "my.bucket.name"},
		{name: "valid with underscores", input: "my_bucket_name"},
		{name: "valid mixed", input: "cluster-1.dev_snapshot"},
		{name: "empty", input: "", wantErr: true},
		{name: "uppercase", input: "My-Bucket", wantErr: true},
		{name: "spaces", input: "my bucket", wantErr: true},
		{name: "semicolon injection", input: "bucket; rm -rf /", wantErr: true},
		{name: "dollar sign", input: "bucket-$HOME", wantErr: true},
		{name: "backtick injection", input: "bucket-`whoami`", wantErr: true},
		{name: "single quotes", input: "bucket'name", wantErr: true},
		{name: "newline", input: "bucket\nname", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input, "test field")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestResolveName(t *testing.T) {
	tests := []struct {
		name     string
		template string
		ctx      map[string]any
		want     string
		wantErr  bool
	}{
		{
			name:     "simple variable",
			template: "{{ .ClusterName }}-bucket",
			ctx:      map[string]any{"ClusterName": "my-cluster"},
			want:     "my-cluster-bucket",
		},
		{
			name:     "multiple variables",
			template: "{{ .ClusterName }}-{{ .PlanId }}-dev",
			ctx:      map[string]any{"ClusterName": "cluster", "PlanId": "plan1"},
			want:     "cluster-plan1-dev",
		},
		{
			name:     "no variables",
			template: "static-bucket-name",
			ctx:      map[string]any{},
			want:     "static-bucket-name",
		},
		{
			name:     "missing variable produces no value",
			template: "{{ .Missing }}-bucket",
			ctx:      map[string]any{},
			want:     "<no value>-bucket",
		},
		{
			name:     "invalid template syntax",
			template: "{{ .Broken",
			ctx:      map[string]any{},
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveName(tt.template, tt.ctx)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGCSManager_serviceAccountName(t *testing.T) {
	tests := []struct {
		name       string
		bucketName string
		want       string
	}{
		{
			name:       "short name",
			bucketName: "my-bucket",
			want:       "eck-bkt-my-bucket",
		},
		{
			name:       "exactly at 30 char limit",
			bucketName: strings.Repeat("a", 22), // "eck-bkt-" (8) + 22 = 30
			want:       "eck-bkt-" + strings.Repeat("a", 22),
		},
		{
			name:       "truncated to 30 chars",
			bucketName: strings.Repeat("a", 30), // "eck-bkt-" (8) + 30 = 38 → truncated
			want:       "eck-bkt-" + strings.Repeat("a", 22),
		},
		{
			name:       "trailing hyphens removed after truncation",
			bucketName: strings.Repeat("a", 21) + "-x", // "eck-bkt-" + 21*a + "-x" = 31 → truncated to "eck-bkt-" + 21*a + "-" → trailing hyphen removed
			want:       "eck-bkt-" + strings.Repeat("a", 21),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GCSManager{cfg: Config{Name: tt.bucketName}}
			got := g.serviceAccountName()
			assert.Equal(t, tt.want, got)
			assert.LessOrEqual(t, len(got), 30, "GCP service account names must be at most 30 characters")
		})
	}
}

func Test_S3Manager_iamUserName(t *testing.T) {
	tests := []struct {
		name       string
		bucketName string
		want       string
	}{
		{
			name:       "short name",
			bucketName: "my-bucket",
			want:       "eck-bkt-my-bucket-storage",
		},
		{
			name:       "exactly at 64 char limit",
			bucketName: strings.Repeat("a", 48), // "eck-bkt-" (8) + 48 + "-storage" (8) = 64
			want:       "eck-bkt-" + strings.Repeat("a", 48) + "-storage",
		},
		{
			name:       "truncated preserves suffix",
			bucketName: strings.Repeat("a", 60), // would be 76 without truncation
			want:       "eck-bkt-" + strings.Repeat("a", 48) + "-storage",
		},
		{
			name:       "very long name still has suffix",
			bucketName: strings.Repeat("x", 200),
			want:       "eck-bkt-" + strings.Repeat("x", 48) + "-storage",
		},
		{
			name:       "empty bucket name",
			bucketName: "",
			want:       "eck-bkt--storage",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &S3Manager{cfg: Config{Name: tt.bucketName}}
			got := s.iamUserName()
			assert.Equal(t, tt.want, got)
			assert.LessOrEqual(t, len(got), 64, "IAM user names must be at most 64 characters")
			assert.True(t, strings.HasSuffix(got, "-storage"), "IAM user name must end with -storage for ownership verification")
		})
	}
}
