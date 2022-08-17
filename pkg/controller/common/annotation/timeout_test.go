// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package annotation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExtractTimeout(t *testing.T) {
	defaultValue := 42 * time.Second
	key := "timeout-annotation"

	testCases := []struct {
		name        string
		annotations map[string]string
		want        time.Duration
	}{
		{
			name: "nil annotations",
			want: defaultValue,
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			want:        defaultValue,
		},
		{
			name:        "missing annotation",
			annotations: map[string]string{"wibble": "wobble"},
			want:        defaultValue,
		},
		{
			name:        "invalid timeout value",
			annotations: map[string]string{"wibble": "wobble", key: "wubble"},
			want:        defaultValue,
		},
		{
			name:        "valid timeout value",
			annotations: map[string]string{"wibble": "wobble", key: "25s"},
			want:        25 * time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			objMeta := metav1.ObjectMeta{
				Name:        "test",
				Annotations: tc.annotations,
			}

			have := ExtractTimeout(context.Background(), objMeta, key, defaultValue)
			require.Equal(t, tc.want, have)
		})
	}
}
