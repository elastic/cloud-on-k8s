// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package maps

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/maps/v1alpha1"
)

func TestNewPodSpec(t *testing.T) {
	tests := []struct {
		name                string
		ems                 emsv1alpha1.ElasticMapsServer
		wantCommandOverride bool
		expectedCommand     []string
	}{
		{
			name: "version 8.9.0 - no command override",
			ems: emsv1alpha1.ElasticMapsServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ems",
					Namespace: "default",
				},
				Spec: emsv1alpha1.MapsSpec{
					Version: "8.9.0",
				},
			},
			wantCommandOverride: false,
		},
		{
			name: "version 9.0.0 - should apply command override",
			ems: emsv1alpha1.ElasticMapsServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ems",
					Namespace: "default",
				},
				Spec: emsv1alpha1.MapsSpec{
					Version: "9.0.0",
				},
			},
			wantCommandOverride: true,
			expectedCommand:     []string{"/bin/sh", "-c", "node app/index.js"},
		},
		{
			name: "version 9.0.1 - should apply command override",
			ems: emsv1alpha1.ElasticMapsServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ems",
					Namespace: "default",
				},
				Spec: emsv1alpha1.MapsSpec{
					Version: "9.0.1",
				},
			},
			wantCommandOverride: true,
			expectedCommand:     []string{"/bin/sh", "-c", "node app/index.js"},
		},
		{
			name: "version 9.1.0 - no command override",
			ems: emsv1alpha1.ElasticMapsServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ems",
					Namespace: "default",
				},
				Spec: emsv1alpha1.MapsSpec{
					Version: "9.1.0",
				},
			},
			wantCommandOverride: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podSpec, err := newPodSpec(tt.ems, "test-hash")
			require.NoError(t, err)

			// Find the main container
			var mapsContainer *corev1.Container
			for _, container := range podSpec.Spec.Containers {
				if container.Name == emsv1alpha1.MapsContainerName {
					mapsContainer = &container
					break
				}
			}

			require.NotNil(t, mapsContainer, "Maps container not found")

			if tt.wantCommandOverride {
				assert.Equal(t, tt.expectedCommand, mapsContainer.Command,
					"Command override doesn't match expected value")
			} else {
				assert.Empty(t, mapsContainer.Command,
					"Command should not be set for versions outside the override range")
			}
		})
	}
}
