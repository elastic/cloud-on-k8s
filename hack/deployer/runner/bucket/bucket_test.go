// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bucket

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
