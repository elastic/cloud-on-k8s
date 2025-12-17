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
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
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

func TestCheckUBISupport(t *testing.T) {
	tests := []struct {
		name         string
		customImage  string
		version      string
		defaultImage string
		wantErr      bool
	}{
		// Custom image tests - should always pass
		{
			name:         "custom image without UBI suffix",
			customImage:  "my-custom-registry/package-registry:8.0.0",
			version:      "8.0.0",
			defaultImage: "registry/image-ubi",
			wantErr:      false,
		},
		{
			name:         "custom image with UBI suffix and unsupported version",
			customImage:  "my-custom-registry/package-registry:9.0.0-ubi",
			version:      "9.0.0",
			defaultImage: "registry/image-ubi",
			wantErr:      false,
		},
		// Version 8.x tests with UBI suffix
		{
			name:         "8.19.7 with UBI - not supported",
			customImage:  "",
			version:      "8.19.7",
			defaultImage: "registry/image-ubi",
			wantErr:      true,
		},
		{
			name:         "8.19.7 without UBI - supported",
			customImage:  "",
			version:      "8.19.7",
			defaultImage: "registry/image",
			wantErr:      false,
		},
		{
			name:         "8.19.8 with UBI - supported",
			customImage:  "",
			version:      "8.19.8",
			defaultImage: "registry/image-ubi",
			wantErr:      false,
		},
		{
			name:         "8.19.9 with UBI - supported",
			customImage:  "",
			version:      "8.19.9",
			defaultImage: "registry/image-ubi",
			wantErr:      false,
		},
		{
			name:         "8.20.0 with UBI - supported",
			customImage:  "",
			version:      "8.20.0",
			defaultImage: "registry/image-ubi",
			wantErr:      false,
		},
		{
			name:         "8.18.0 with UBI - not supported",
			customImage:  "",
			version:      "8.18.0",
			defaultImage: "registry/image-ubi",
			wantErr:      true,
		},
		{
			name:         "8.18.0 without UBI - supported",
			customImage:  "",
			version:      "8.18.0",
			defaultImage: "registry/image",
			wantErr:      false,
		},
		// Version 9.0.x tests
		{
			name:         "9.0.3 with UBI - not supported",
			customImage:  "",
			version:      "9.0.3",
			defaultImage: "registry/image-ubi",
			wantErr:      true,
		},
		{
			name:         "9.0.3 without UBI - supported",
			customImage:  "",
			version:      "9.0.3",
			defaultImage: "registry/image",
			wantErr:      false,
		},
		// Version 9.1.x tests
		{
			name:         "9.1.2 without UBI - supported",
			customImage:  "",
			version:      "9.1.2",
			defaultImage: "registry/image",
			wantErr:      false,
		},
		{
			name:         "9.1.7 with UBI - not supported",
			customImage:  "",
			version:      "9.1.7",
			defaultImage: "registry/image-ubi",
			wantErr:      true,
		},
		{
			name:         "9.1.8 with UBI - supported (minimum for 9.1.x)",
			customImage:  "",
			version:      "9.1.8",
			defaultImage: "registry/image-ubi",
			wantErr:      false,
		},
		{
			name:         "9.1.9 with UBI - supported",
			customImage:  "",
			version:      "9.1.9",
			defaultImage: "registry/image-ubi",
			wantErr:      false,
		},
		// Version 9.2.x tests
		{
			name:         "9.2.0 with UBI - not supported",
			customImage:  "",
			version:      "9.2.0",
			defaultImage: "registry/image-ubi",
			wantErr:      true,
		},
		{
			name:         "9.2.0 without UBI - supported",
			customImage:  "",
			version:      "9.2.0",
			defaultImage: "registry/image",
			wantErr:      false,
		},
		{
			name:         "9.2.1 with UBI - not supported",
			customImage:  "",
			version:      "9.2.1",
			defaultImage: "registry/image-ubi",
			wantErr:      true,
		},
		{
			name:         "9.2.2 with UBI - supported (minimum for 9.2+)",
			customImage:  "",
			version:      "9.2.2",
			defaultImage: "registry/image-ubi",
			wantErr:      false,
		},
		{
			name:         "9.2.3 with UBI - supported",
			customImage:  "",
			version:      "9.2.3",
			defaultImage: "registry/image-ubi",
			wantErr:      false,
		},
		{
			name:         "9.3.0 with UBI - supported",
			customImage:  "",
			version:      "9.3.0",
			defaultImage: "registry/image-ubi",
			wantErr:      false,
		},
		// Version 7.x tests with UBI suffix
		{
			name:         "7.17.0 with UBI - not supported",
			customImage:  "",
			version:      "7.17.0",
			defaultImage: "registry/image-ubi",
			wantErr:      true,
		},
		// Version 7.x tests without UBI suffix
		{
			name:         "7.17.0 with UBI - not supported",
			customImage:  "",
			version:      "7.17.0",
			defaultImage: "registry/image",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := version.Parse(tt.version)
			require.NoError(t, err, "failed to parse version")

			err = checkUBISupport(tt.customImage, tt.defaultImage, v)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
