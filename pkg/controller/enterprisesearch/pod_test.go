// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package enterprisesearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/enterprisesearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
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

func Test_withESCertsVolume(t *testing.T) {
	tests := []struct {
		name                 string
		conf                 commonv1.AssociationConf
		wantCAVolume         bool
		wantClientCertVolume bool
	}{
		{
			name: "CA and client cert both present",
			conf: commonv1.AssociationConf{
				AuthSecretName:       "auth-secret",
				AuthSecretKey:        "elastic",
				CACertProvided:       true,
				CASecretName:         "ca-secret",
				URL:                  "https://es:9200",
				ClientCertSecretName: "client-cert-secret",
			},
			wantCAVolume:         true,
			wantClientCertVolume: true,
		},
		{
			name: "client cert only, no CA",
			conf: commonv1.AssociationConf{
				AuthSecretName:       "auth-secret",
				AuthSecretKey:        "elastic",
				CACertProvided:       false,
				URL:                  "https://es:9200",
				ClientCertSecretName: "client-cert-secret",
			},
			wantCAVolume:         false,
			wantClientCertVolume: true,
		},
		{
			name: "neither client cert nor CA",
			conf: commonv1.AssociationConf{
				AuthSecretName:       "auth-secret",
				AuthSecretKey:        "elastic",
				CACertProvided:       false,
				URL:                  "https://es:9200",
				ClientCertSecretName: "",
			},
			wantCAVolume:         false,
			wantClientCertVolume: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ent := entv1.EnterpriseSearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fake-ent",
					Namespace: "default",
				},
				Spec: entv1.EnterpriseSearchSpec{
					Version:          "8.0.0",
					ElasticsearchRef: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es", Namespace: "default"}},
				},
			}
			ent.SetAssociationConf(&tt.conf)

			builder := defaults.NewPodTemplateBuilder(corev1.PodTemplateSpec{}, entv1.EnterpriseSearchContainerName)
			builder, err := withESCertsVolume(builder, ent)
			assert.NoError(t, err)

			pod := builder.PodTemplate
			var hasCAVolume, hasClientCertVolume bool
			for _, vol := range pod.Spec.Volumes {
				if vol.Secret != nil && vol.Secret.SecretName == "ca-secret" {
					hasCAVolume = true
				}
				if vol.Secret != nil && vol.Secret.SecretName == "client-cert-secret" {
					hasClientCertVolume = true
				}
			}
			assert.Equal(t, tt.wantCAVolume, hasCAVolume, "CA volume")
			assert.Equal(t, tt.wantClientCertVolume, hasClientCertVolume, "client cert volume")
		})
	}
}
