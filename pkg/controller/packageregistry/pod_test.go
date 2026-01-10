// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package packageregistry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
)

func TestNewPodSpec(t *testing.T) {
	tests := []struct {
		name       string
		epr        eprv1alpha1.PackageRegistry
		assertions func(pod *corev1.PodTemplateSpec)
	}{
		{
			name: "version 9.3.0 should have runAsNonRoot set to true",
			epr: eprv1alpha1.PackageRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-epr",
					Namespace: "default",
				},
				Spec: eprv1alpha1.PackageRegistrySpec{
					Version: "9.3.0",
				},
			},
			assertions: func(pod *corev1.PodTemplateSpec) {
				require.Len(t, pod.Spec.Containers, 1)
				container := pod.Spec.Containers[0]
				require.NotNil(t, container.SecurityContext)
				require.NotNil(t, container.SecurityContext.RunAsNonRoot)
				assert.True(t, *container.SecurityContext.RunAsNonRoot)
			},
		},
		{
			name: "version 9.2.0 should have runAsNonRoot set to nil",
			epr: eprv1alpha1.PackageRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-epr",
					Namespace: "default",
				},
				Spec: eprv1alpha1.PackageRegistrySpec{
					Version: "9.2.0",
				},
			},
			assertions: func(pod *corev1.PodTemplateSpec) {
				require.Len(t, pod.Spec.Containers, 1)
				container := pod.Spec.Containers[0]
				require.NotNil(t, container.SecurityContext)
				assert.Nil(t, container.SecurityContext.RunAsNonRoot)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := metadata.Metadata{
				Labels: map[string]string{
					"test-label": "test-value",
				},
			}
			got, err := newPodSpec(tt.epr, "test-config-hash", md)
			require.NoError(t, err)
			tt.assertions(&got)
		})
	}
}
