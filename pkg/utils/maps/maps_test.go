// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package maps

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSubset(t *testing.T) {
	tests := []struct {
		name string
		map1 map[string]string
		map2 map[string]string
		want bool
	}{
		{
			name: "when map1 is nil",
			map2: map[string]string{"x": "y"},
			want: true,
		},
		{
			name: "when map2 is nil",
			map1: map[string]string{"x": "y"},
			want: false,
		},
		{
			name: "when both maps are nil",
			want: true,
		},
		{
			name: "when map1 is empty",
			map1: map[string]string{},
			map2: map[string]string{"x": "y"},
			want: true,
		},
		{
			name: "when map2 is empty",
			map1: map[string]string{"x": "y"},
			map2: map[string]string{},
			want: false,
		},
		{
			name: "when both maps are empty",
			map1: map[string]string{},
			map2: map[string]string{},
			want: true,
		},
		{
			name: "when both maps contain the same items",
			map1: map[string]string{"x": "y", "a": "b"},
			map2: map[string]string{"x": "y", "a": "b"},
			want: true,
		},
		{
			name: "when keys are the same but value are different",
			map1: map[string]string{"x": "p", "a": "q"},
			map2: map[string]string{"x": "y", "a": "b"},
			want: false,
		},

		{
			name: "when map1 has fewer items than map2",
			map1: map[string]string{"x": "y"},
			map2: map[string]string{"x": "y", "a": "b"},
			want: true,
		},
		{
			name: "when map1 has more items than map2",
			map1: map[string]string{"x": "y", "a": "b"},
			map2: map[string]string{"x": "y"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			have := IsSubset(tt.map1, tt.map2)
			require.Equal(t, tt.want, have)
		})
	}
}

func TestMerge(t *testing.T) {
	tests := []struct {
		name string
		dest map[string]string
		src  map[string]string
		want map[string]string
	}{
		{
			name: "when dest is nil",
			src:  map[string]string{"x": "y"},
			want: map[string]string{"x": "y"},
		},
		{
			name: "when src is nil",
			dest: map[string]string{"x": "y"},
			want: map[string]string{"x": "y"},
		},
		{
			name: "when both maps are nil",
		},
		{
			name: "when dest is empty",
			dest: map[string]string{},
			src:  map[string]string{"x": "y"},
			want: map[string]string{"x": "y"},
		},
		{
			name: "when src is empty",
			dest: map[string]string{"x": "y"},
			src:  map[string]string{},
			want: map[string]string{"x": "y"},
		},
		{
			name: "when both maps are empty",
			dest: map[string]string{},
			src:  map[string]string{},
			want: map[string]string{},
		},
		{
			name: "when both maps contain the same items",
			dest: map[string]string{"x": "y", "a": "b"},
			src:  map[string]string{"x": "y", "a": "b"},
			want: map[string]string{"x": "y", "a": "b"},
		},
		{
			name: "when keys are the same but value are different",
			dest: map[string]string{"x": "p", "a": "q"},
			src:  map[string]string{"x": "y", "a": "b"},
			want: map[string]string{"x": "y", "a": "b"},
		},

		{
			name: "when dest has fewer items than src",
			dest: map[string]string{"x": "y"},
			src:  map[string]string{"x": "y", "a": "b"},
			want: map[string]string{"x": "y", "a": "b"},
		},
		{
			name: "when dest has more items than src",
			dest: map[string]string{"x": "y", "a": "b"},
			src:  map[string]string{"x": "y"},
			want: map[string]string{"x": "y", "a": "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			have := Merge(tt.dest, tt.src)
			require.Equal(t, tt.want, have)
		})
	}
}
