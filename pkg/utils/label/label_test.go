// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package label

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHasLabel(t *testing.T) {
	tests := []struct {
		name   string
		object metav1.Object
		labels []string
		want   bool
	}{
		{
			name: "no labels on object",
			object: &metav1.ObjectMeta{
				Labels: map[string]string{},
			},
			labels: []string{"x", "y"},
			want:   false,
		},
		{
			// empty label set is a subset of every non-empty set
			name: "empty label set provided",
			object: &metav1.ObjectMeta{
				Labels: map[string]string{"x": "y"},
			},
			labels: []string{},
			want:   true,
		},
		{
			name: "labels that match",
			object: &metav1.ObjectMeta{
				Labels: map[string]string{"x": "y", "a": "b"},
			},
			labels: []string{"x", "a"},
			want:   true,
		},
		{
			name: "labels that don't match",
			object: &metav1.ObjectMeta{
				Labels: map[string]string{"x": "y", "a": "b"},
			},
			labels: []string{"c", "d"},
			want:   false,
		},
		{
			name: "labels that's nil",
			object: &metav1.ObjectMeta{
				Labels: nil,
			},
			labels: []string{"c", "d"},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			have := HasLabel(tc.object, tc.labels...)
			require.Equal(t, tc.want, have)
		})
	}
}
