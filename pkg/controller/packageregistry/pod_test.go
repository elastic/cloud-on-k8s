// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package packageregistry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
)

func TestNewPodSpec(t *testing.T) {
	getEprWithVersion := func(version string) eprv1alpha1.PackageRegistry {
		return eprv1alpha1.PackageRegistry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-epr",
				Namespace: "default",
			},
			Spec: eprv1alpha1.PackageRegistrySpec{
				Version: version,
			},
		}
	}

	tests := []struct {
		name               string
		epr                eprv1alpha1.PackageRegistry
		expectRunAsNonRoot bool
	}{
		// Versions >= 9.3.0 should have runAsNonRoot = true
		{
			name:               "version 9.3.0 should have runAsNonRoot set to true",
			epr:                getEprWithVersion("9.3.0"),
			expectRunAsNonRoot: true,
		},
		{
			name:               "version 9.4.0 should have runAsNonRoot set to true",
			epr:                getEprWithVersion("9.4.0"),
			expectRunAsNonRoot: true,
		},
		{
			name:               "version 10.0.0 should have runAsNonRoot set to true",
			epr:                getEprWithVersion("10.0.0"),
			expectRunAsNonRoot: true,
		},
		// Versions 9.2.x where x >= 4 should have runAsNonRoot = true
		{
			name:               "version 9.2.4 should have runAsNonRoot set to true",
			epr:                getEprWithVersion("9.2.4"),
			expectRunAsNonRoot: true,
		},
		{
			name:               "version 9.2.5 should have runAsNonRoot set to true",
			epr:                getEprWithVersion("9.2.5"),
			expectRunAsNonRoot: true,
		},
		{
			name:               "version 9.2.3 should have runAsNonRoot set to nil",
			epr:                getEprWithVersion("9.2.3"),
			expectRunAsNonRoot: false,
		},
		// Versions 9.1.x where x >= 10 should have runAsNonRoot = true
		{
			name:               "version 9.1.10 should have runAsNonRoot set to true",
			epr:                getEprWithVersion("9.1.10"),
			expectRunAsNonRoot: true,
		},
		{
			name:               "version 9.1.11 should have runAsNonRoot set to true",
			epr:                getEprWithVersion("9.1.11"),
			expectRunAsNonRoot: true,
		},
		{
			name:               "version 9.1.9 should have runAsNonRoot set to nil",
			epr:                getEprWithVersion("9.1.9"),
			expectRunAsNonRoot: false,
		},
		// Versions 8.19.x where x >= 10 should have runAsNonRoot = true
		{
			name:               "version 8.19.10 should have runAsNonRoot set to true",
			epr:                getEprWithVersion("8.19.10"),
			expectRunAsNonRoot: true,
		},
		{
			name:               "version 8.19.11 should have runAsNonRoot set to true",
			epr:                getEprWithVersion("8.19.11"),
			expectRunAsNonRoot: true,
		},
		{
			name:               "version 8.19.9 should have runAsNonRoot set to nil",
			epr:                getEprWithVersion("8.19.9"),
			expectRunAsNonRoot: false,
		},
		// Other 9.0.x versions should have runAsNonRoot = nil
		{
			name:               "version 9.0.0 should have runAsNonRoot set to nil",
			epr:                getEprWithVersion("9.0.0"),
			expectRunAsNonRoot: false,
		},
		{
			name:               "version 9.0.5 should have runAsNonRoot set to nil",
			epr:                getEprWithVersion("9.0.5"),
			expectRunAsNonRoot: false,
		},
		// Other 8.x versions should have runAsNonRoot = nil
		{
			name:               "version 8.18.0 should have runAsNonRoot set to nil",
			epr:                getEprWithVersion("8.18.0"),
			expectRunAsNonRoot: false,
		},
		{
			name:               "version 8.20.0 should have runAsNonRoot set to true",
			epr:                getEprWithVersion("8.20.0"),
			expectRunAsNonRoot: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := metadata.Metadata{
				Labels: map[string]string{
					"test-label": "test-value",
				},
			}
			podSpec, err := newPodSpec(tt.epr, "test-config-hash", md, true)
			require.NoError(t, err)
			require.Len(t, podSpec.Spec.Containers, 1)
			container := podSpec.Spec.Containers[0]
			require.NotNil(t, container.SecurityContext)
			if tt.expectRunAsNonRoot {
				assert.NotNil(t, container.SecurityContext.RunAsNonRoot)
				assert.True(t, *container.SecurityContext.RunAsNonRoot)
			} else {
				assert.Nil(t, container.SecurityContext.RunAsNonRoot)
			}
		})
	}
}
