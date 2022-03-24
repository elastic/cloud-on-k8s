// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package enterprisesearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	entv1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1"
)

func Test_newPodSpec(t *testing.T) {
	tests := []struct {
		name       string
		ent        entv1.EnterpriseSearch
		assertions func(pod corev1.PodTemplateSpec)
	}{
		{
			name: "user-provided init containers should inherit from the default main container image",
			ent: entv1.EnterpriseSearch{
				Spec: entv1.EnterpriseSearchSpec{
					Version: "7.8.0",
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name: "user-init-container",
								},
							},
						},
					},
				}},
			assertions: func(pod corev1.PodTemplateSpec) {
				assert.Len(t, pod.Spec.InitContainers, 1)
				assert.Equal(t, pod.Spec.Containers[0].Image, pod.Spec.InitContainers[0].Image)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newPodSpec(tt.ent, "amFpbWVsZXNjaGF0c2V0dm91cz8=")
			assert.NoError(t, err)
			tt.assertions(got)
		})
	}
}
