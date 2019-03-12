/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package pod

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
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
