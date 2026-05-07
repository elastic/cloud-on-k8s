// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package annotation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestHasClientAuthenticationRequired(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{
			name:        "returns true when annotation is set to true",
			annotations: map[string]string{ClientAuthenticationRequiredAnnotation: "true"},
			want:        true,
		},
		{
			name: "returns false when annotation is absent",
			want: false,
		},
		{
			name:        "returns false when annotation has unexpected value",
			annotations: map[string]string{ClientAuthenticationRequiredAnnotation: "false"},
			want:        false,
		},
		{
			name:        "returns false when annotation is empty string",
			annotations: map[string]string{ClientAuthenticationRequiredAnnotation: ""},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations}}
			require.Equal(t, tt.want, HasClientAuthenticationRequired(obj))
		})
	}
}

func TestSetClientAuthenticationRequiredAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
	}{
		{
			name: "sets annotation on object with no annotations",
		},
		{
			name:        "no-op when annotation already present",
			annotations: map[string]string{ClientAuthenticationRequiredAnnotation: "true"},
		},
		{
			name:        "overwrites incorrect annotation value",
			annotations: map[string]string{ClientAuthenticationRequiredAnnotation: "false"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Namespace:   "ns",
				Name:        "test",
				Annotations: tt.annotations,
			}}
			c := k8s.NewFakeClient(obj)

			require.NoError(t, SetClientAuthenticationRequiredAnnotation(ctx, c, obj))

			var updated corev1.Secret
			require.NoError(t, c.Get(ctx, types.NamespacedName{Namespace: "ns", Name: "test"}, &updated))
			require.Equal(t, "true", updated.Annotations[ClientAuthenticationRequiredAnnotation])
		})
	}
}

func TestRemoveClientAuthenticationRequiredAnnotation(t *testing.T) {
	tests := []struct {
		name                 string
		annotations          map[string]string
		wantAnnotationAbsent bool
	}{
		{
			name:                 "removes annotation when present",
			annotations:          map[string]string{ClientAuthenticationRequiredAnnotation: "true"},
			wantAnnotationAbsent: true,
		},
		{
			name:                 "no-op when annotation already absent",
			wantAnnotationAbsent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Namespace:   "ns",
				Name:        "test",
				Annotations: tt.annotations,
			}}
			c := k8s.NewFakeClient(obj)

			require.NoError(t, RemoveClientAuthenticationRequiredAnnotation(ctx, c, obj))

			var updated corev1.Secret
			require.NoError(t, c.Get(ctx, types.NamespacedName{Namespace: "ns", Name: "test"}, &updated))
			require.False(t, HasClientAuthenticationRequired(&updated))
		})
	}
}
