// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestMemFromJavaOpts(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		expected resource.Quantity
		isErr    bool
	}{
		{
			name:     "in k",
			actual:   "-Xms1k -Xmx8388608k",
			expected: resource.MustParse("16777216Ki"),
		},
		{
			name:     "in K",
			actual:   "-Xmx1024K",
			expected: resource.MustParse("2048Ki"),
		},
		{
			name:     "in m",
			actual:   "-Xmx512m -Xms256m",
			expected: resource.MustParse("1024Mi"),
		},
		{
			name:     "in M",
			actual:   "-Xmx256M",
			expected: resource.MustParse("512Mi"),
		},
		{
			name:     "in g",
			actual:   "-Xmx64g",
			expected: resource.MustParse("128Gi"),
		},
		{
			name:     "in G",
			actual:   "-Xmx64G",
			expected: resource.MustParse("128Gi"),
		},
		{
			name:   "without unit",
			actual: "-Xmx83886080",
			isErr:  true,
		},
		{
			name:   "without value",
			actual: "-XmxM",
			isErr:  true,
		},
		{
			name:   "with an invalid Xmx",
			actual: "-XMX1k",
			isErr:  true,
		},
		{
			name:   "with an invalid unit",
			actual: "-Xmx64GB",
			isErr:  true,
		},
		{
			name:     "without xmx",
			actual:   "-Xms1k",
			expected: resource.MustParse("16777216k"),
			isErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := memFromJavaOpts(tt.actual)
			if tt.isErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if !got.Equal(tt.expected) {
					t.Errorf("memFromJavaOpts(%s) = %v, want %s", tt.actual, got.String(), tt.expected.String())
				}
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
				got := resource.MustParse(tt.expected)
				if !got.Equal(q) {
					t.Errorf("memFromNodeOptions(%s) = %v, want %s", tt.actual, got, tt.expected)
				}
			}
		})
	}
}
