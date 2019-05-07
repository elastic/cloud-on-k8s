// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func Test_imageWithVersion(t *testing.T) {
	type args struct {
		image   string
		version string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			args: args{image: "someimage", version: "6.4.2"},
			want: "someimage:6.4.2",
		},
		{
			args: args{image: "differentimage", version: "6.4.1"},
			want: "differentimage:6.4.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := imageWithVersion(tt.args.image, tt.args.version)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewPodSpec(t *testing.T) {
	testSelector := &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: "secret-name",
		},
		Key: "u",
	}
	tests := []struct {
		name       string
		args       SpecParams
		assertions func(params corev1.PodSpec)
	}{
		{
			name: "defaults",
			args: SpecParams{},
			assertions: func(got corev1.PodSpec) {
				expected := imageWithVersion(defaultImageRepositoryAndName, "")
				assert.Equal(t, expected, got.Containers[0].Image)
			},
		},
		{
			name: "overrides",
			args: SpecParams{CustomImageName: "my-custom-image:1.0.0", Version: "7.0.0"},
			assertions: func(got corev1.PodSpec) {
				assert.Equal(t, "my-custom-image:1.0.0", got.Containers[0].Image)
			},
		},
		{
			name: "auth settings inline",
			args: SpecParams{
				User: v1alpha1.ElasticsearchAuth{
					Inline: &v1alpha1.ElasticsearchInlineAuth{
						Username: "u",
						Password: "p",
					},
				},
			},
			assertions: func(got corev1.PodSpec) {
				userName := envWithName(t, elasticsearchUsername, got.Containers[0])
				assert.Equal(t, "u", userName.Value)
				password := envWithName(t, elasticsearchPassword, got.Containers[0])
				assert.Equal(t, "p", password.Value)
			},
		},
		{
			name: "auth settings via secret",
			args: SpecParams{
				User: v1alpha1.ElasticsearchAuth{
					SecretKeyRef: testSelector,
				},
			},
			assertions: func(got corev1.PodSpec) {
				userName := envWithName(t, elasticsearchUsername, got.Containers[0])
				assert.Equal(t, "u", userName.Value)
				password := envWithName(t, elasticsearchPassword, got.Containers[0])
				assert.Equal(t, testSelector, password.ValueFrom.SecretKeyRef)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSpec(tt.args, EnvFactory(func(p SpecParams) []corev1.EnvVar {
				var env []corev1.EnvVar
				return ApplyToEnv(tt.args.User, env) // common across versions for now
			}))
			tt.assertions(got)
		})
	}
}

func envWithName(t *testing.T, name string, container corev1.Container) corev1.EnvVar {
	for _, v := range container.Env {
		if v.Name == name {
			return v
		}
	}
	t.Errorf("expected env var %s does not exist ", name)
	return corev1.EnvVar{}
}

func Test_resourceRequirements(t *testing.T) {
	tests := []struct {
		name        string
		podTemplate corev1.PodTemplateSpec
		want        corev1.ResourceRequirements
	}{
		{
			name:        "empty pod template",
			podTemplate: corev1.PodTemplateSpec{},
			want:        DefaultResources,
		},
		{
			name: "pod template with resource limits for Kibana container",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: v1alpha1.KibanaContainerName,
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("3Gi")},
							},
						},
					},
				},
			},
			want: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("3Gi")},
			},
		},
		{
			name: "pod template with resource limits for another container",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "not kibana container",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("3Gi")},
							},
						},
					},
				},
			},
			want: DefaultResources,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resourceRequirements(tt.podTemplate); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resourceRequirements() = %v, want %v", got, tt.want)
			}
		})
	}
}
