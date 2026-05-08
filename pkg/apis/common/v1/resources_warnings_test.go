// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodTemplateResourcesOverrideWarning(t *testing.T) {
	cpu := resource.MustParse("500m")
	memory := resource.MustParse("1Gi")
	mainContainer := "elasticsearch"

	podTemplate := func(container string, res corev1.ResourceRequirements) corev1.PodTemplateSpec {
		return corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: container, Resources: res}}}}
	}

	tests := []struct {
		name        string
		shorthand   Resources
		podTemplate corev1.PodTemplateSpec
		wantWarning bool
	}{
		{
			name:        "empty shorthand",
			podTemplate: podTemplate(mainContainer, corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: cpu}}),
		},
		{
			name:        "overlap on requests.cpu",
			shorthand:   Resources{Requests: ResourceAllocations{CPU: &cpu}},
			podTemplate: podTemplate(mainContainer, corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: cpu}}),
			wantWarning: true,
		},
		{
			name:        "no overlap: shorthand CPU, pod template memory",
			shorthand:   Resources{Requests: ResourceAllocations{CPU: &cpu}},
			podTemplate: podTemplate(mainContainer, corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceMemory: memory}}),
		},
		{
			name:        "main container missing",
			shorthand:   Resources{Requests: ResourceAllocations{CPU: &cpu}},
			podTemplate: podTemplate("other", corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: cpu}}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PodTemplateResourcesOverrideWarning("spec.resources", "spec.podTemplate", mainContainer, tt.shorthand, tt.podTemplate)
			if tt.wantWarning {
				assert.NotEmpty(t, got)
				return
			}
			assert.Empty(t, got)
		})
	}
}
