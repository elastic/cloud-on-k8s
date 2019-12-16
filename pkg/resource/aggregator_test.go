// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestMemFromJavaOpts(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		expected string
		isErr    bool
	}{
		{
			name:     "in k",
			actual:   "-Xms1k -Xmx8388608k",
			expected: "16777216k",
		},
		{
			name:     "in K",
			actual:   "-Xmx1024K",
			expected: "2048k",
		},
		{
			name:     "in m",
			actual:   "-Xmx512m -Xms256m",
			expected: "1024M",
		},
		{
			name:     "in M",
			actual:   "-Xmx256M",
			expected: "512M",
		},
		{
			name:     "in g",
			actual:   "-Xmx64g",
			expected: "128G",
		},
		{
			name:     "in G",
			actual:   "-Xmx64G",
			expected: "128G",
		},
		{
			name:     "without unit",
			actual:   "-Xmx83886080",
			expected: "167772160",
		},
		{
			name:   "with an invalid Xmx",
			actual: "-XMX1k",
			isErr:  true,
		},
		{
			name:     "without xmx",
			actual:   "-Xms1k",
			expected: "16777216k",
			isErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := memFromJavaOpts(tt.actual)
			if tt.isErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, resource.MustParse(tt.expected).Equal(q))
			}
		})
	}
}

func TestMemFromNodeOpts(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		expected string
		isErr    bool
	}{
		{
			name:     "with max-old-space-size option",
			actual:   "--max-old-space-size=2048",
			expected: "2048M",
		},
		{
			name:   "empty options",
			actual: "",
			isErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := memFromNodeOptions(tt.actual)
			if tt.isErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, resource.MustParse(tt.expected).Equal(q))
			}
		})
	}
}
