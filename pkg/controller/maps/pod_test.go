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
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
)

func TestNewPodSpec_CommandOverride(t *testing.T) {
	// Command to use for the OpenShift workaround
	commandOverride := []string{"/bin/sh", "-c", "node app/index.js"}

	tests := []struct {
		name                      string
		version                   string
		setDefaultSecurityContext bool
		wantCommandOverride       bool
		expectedCommand           []string
	}{
		// 7.x version tests
		{
			name:                "version 7.17.27 - no command override",
			version:             "7.17.27",
			wantCommandOverride: false,
		},
		{
			name:                "version 7.17.28 - should apply command override",
			version:             "7.17.28",
			wantCommandOverride: true,
			expectedCommand:     commandOverride,
		},
		{
			name:                "version 7.17.29 - should not apply command override",
			version:             "7.17.29",
			wantCommandOverride: false,
		},
		// 8.x version tests
		{
			name:                "version 8.17.6 - no command override",
			version:             "8.17.6",
			wantCommandOverride: false,
		},
		{
			name:                "version 8.18.0 - should apply command override",
			version:             "8.18.0",
			wantCommandOverride: true,
			expectedCommand:     commandOverride,
		},
		{
			name:                "version 8.18.1 - should apply command override",
			version:             "8.18.1",
			wantCommandOverride: true,
			expectedCommand:     commandOverride,
		},
		{
			name:                "version 8.19.0 - should not apply command override",
			version:             "8.19.0",
			wantCommandOverride: false,
		},
		// 9.x version tests
		{
			name:                "version 9.0.0 - should apply command override",
			version:             "9.0.0",
			wantCommandOverride: true,
			expectedCommand:     commandOverride,
		},
		{
			name:                "version 9.1.0 - should not apply command override",
			version:             "9.1.0",
			wantCommandOverride: false,
		},
		// Security context tests
		{
			name:                      "setDefaultSecurityContext enabled",
			version:                   "9.1.0",
			setDefaultSecurityContext: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test EMS with the specified version
			ems := emsv1alpha1.ElasticMapsServer{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ems", Namespace: "default"},
				Spec:       emsv1alpha1.MapsSpec{Version: tt.version},
			}

			podSpec, err := newPodSpec(ems, "test-hash", metadata.Metadata{}, tt.setDefaultSecurityContext)
			require.NoError(t, err)

			// Find the main container
			var mapsContainer *corev1.Container
			for i := range podSpec.Spec.Containers {
				if podSpec.Spec.Containers[i].Name == emsv1alpha1.MapsContainerName {
					mapsContainer = &podSpec.Spec.Containers[i]
					break
				}
			}

			require.NotNil(t, mapsContainer, "Maps container not found")

			if tt.wantCommandOverride {
				assert.Equal(t, tt.expectedCommand, mapsContainer.Command,
					"Command override doesn't match expected value for version %s", tt.version)
			} else {
				assert.Empty(t, mapsContainer.Command,
					"Command should not be set for version %s", tt.version)
			}

			if tt.setDefaultSecurityContext {
				require.NotNil(t, podSpec.Spec.SecurityContext, "PodSecurityContext should be set")
				require.NotNil(t, podSpec.Spec.SecurityContext.SeccompProfile, "SeccompProfile should be set")
				assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, podSpec.Spec.SecurityContext.SeccompProfile.Type,
					"SeccompProfile type should be RuntimeDefault")
			}

		})
	}
}
