// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen1

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
)

var testProbeUser = client.UserAuth{Name: "username1", Password: "supersecure"}

func TestNewEnvironmentVars(t *testing.T) {
	type args struct {
		p pod.NewPodSpecParams
	}
	tests := []struct {
		name    string
		args    args
		wantEnv []corev1.EnvVar
	}{
		{
			name: "sample cluster",
			args: args{
				p: pod.NewPodSpecParams{
					ProbeUser: testProbeUser,
					Elasticsearch: v1alpha1.Elasticsearch{
						Spec: v1alpha1.ElasticsearchSpec{
							Version: "7.1.0",
						},
					},
				},
			},
			wantEnv: []corev1.EnvVar{
				{Name: settings.EnvReadinessProbeProtocol, Value: "https"},
				{Name: settings.EnvProbeUsername, Value: "username1"},
				{Name: settings.EnvProbePasswordFile, Value: path.Join(esvolume.ProbeUserSecretMountPath, "username1")},
				{Name: settings.EnvPodName, Value: "", ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
				}},
				{Name: settings.EnvPodIP, Value: "", ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
				}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewEnvironmentVars(tt.args.p)
			assert.Equal(t, tt.wantEnv, got)
		})
	}
}
