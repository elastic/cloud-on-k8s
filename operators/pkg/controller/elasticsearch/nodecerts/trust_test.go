// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

//+ build integration

package nodecerts

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestTrustRootConfig_Include(t *testing.T) {
	tests := []struct {
		name              string
		trustRootConfig   TrustRootConfig
		trustRestrictions v1alpha1.TrustRestrictions
		expected          TrustRootConfig
	}{
		{
			name:            "include new subject",
			trustRootConfig: TrustRootConfig{Trust: TrustConfig{SubjectName: []string{"foo"}}},
			trustRestrictions: v1alpha1.TrustRestrictions{
				Trust: v1alpha1.Trust{
					SubjectName: []string{"bar"},
				},
			},
			expected: TrustRootConfig{
				Trust: TrustConfig{SubjectName: []string{"foo", "bar"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.trustRootConfig.Include(tt.trustRestrictions)
			assert.Equal(t, tt.trustRootConfig, tt.expected)
		})
	}
}
