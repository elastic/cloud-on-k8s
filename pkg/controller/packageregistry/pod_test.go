// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package packageregistry

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
)

func TestNewPodSpec_CommandOverride(t *testing.T) {
	tests := []struct {
		name            string
		version         string
		expectedCommand []string
	}{
		// 7.x version tests
		{
			name:    "version 7.17.27 - no command override",
			version: "7.17.27",
		},
		{
			name:    "version 7.17.28 - should apply command override",
			version: "7.17.28",
		},
		{
			name:    "version 7.17.29 - should not apply command override",
			version: "7.17.29",
		},
		// 8.x version tests
		{
			name:    "version 8.17.6 - no command override",
			version: "8.17.6",
		},
		{
			name:    "version 8.18.0 - should apply command override",
			version: "8.18.0",
		},
		{
			name:    "version 8.18.1 - should apply command override",
			version: "8.18.1",
		},
		{
			name:    "version 8.19.0 - should not apply command override",
			version: "8.19.0",
		},
		// 9.x version tests
		{
			name:    "version 9.0.0 - should apply command override",
			version: "9.0.0",
		},
		{
			name:    "version 9.1.0 - should not apply command override",
			version: "9.1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			epr := eprv1alpha1.PackageRegistry{
				ObjectMeta: metav1.ObjectMeta{Name: "test-epr", Namespace: "default"},
				Spec:       eprv1alpha1.PackageRegistrySpec{Version: tt.version},
			}

			podSpec, err := newPodSpec(epr, "test-hash", metadata.Metadata{}, false)
			require.NoError(t, err)

			// Find the main container
			var eprContainer *corev1.Container
			for i := range podSpec.Spec.Containers {
				if podSpec.Spec.Containers[i].Name == eprv1alpha1.EPRContainerName {
					eprContainer = &podSpec.Spec.Containers[i]
					break
				}
			}

			require.NotNil(t, eprContainer, "EPR container not found")
		})
	}
}
