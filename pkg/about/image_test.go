// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package about

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOperatorImage(t *testing.T) {
	const image = "docker.elastic.co/eck/eck-operator:3.4.1"

	tests := []struct {
		name    string
		envVal  string
		want    string
		wantErr bool
	}{
		{
			name:   "returns image from OPERATOR_IMAGE env var",
			envVal: image,
			want:   image,
		},
		{
			name:    "error when OPERATOR_IMAGE is not set",
			envVal:  "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Setenv("OPERATOR_IMAGE", tt.envVal)
		got, err := GetOperatorImageFromEnv()
		if tt.wantErr {
			require.Error(t, err, tt.name)
			continue
		}
		require.NoError(t, err, tt.name)
		assert.Equal(t, tt.want, got, tt.name)
	}
}
