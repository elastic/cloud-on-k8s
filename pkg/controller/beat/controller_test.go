// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"context"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/beat"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

func Test_newDriverParams(t *testing.T) {
	for _, tt := range []struct {
		name           string
		deployment     *v1beta1.DeploymentSpec
		wantDeployment *beat.DeploymentSpec
		daemonSet      *v1beta1.DaemonSetSpec
		wantDaemonSet  *beat.DaemonSetSpec
	}{
		{
			name: "without deployment/daemonset",
		},
		{
			name:           "with empty deployment",
			deployment:     &v1beta1.DeploymentSpec{},
			wantDeployment: &beat.DeploymentSpec{},
		},
		{
			name:           "with replicas in deployment",
			deployment:     &v1beta1.DeploymentSpec{Replicas: pointer.Int32(2)},
			wantDeployment: &beat.DeploymentSpec{Replicas: pointer.Int32(2)},
		},
		{
			name: "with deployment partial podspec",
			deployment: &v1beta1.DeploymentSpec{PodTemplate: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					ServiceAccountName: "sa-test",
				},
			}},
			wantDeployment: &beat.DeploymentSpec{PodTemplate: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					ServiceAccountName: "sa-test",
				},
			}},
		},
		{
			name: "with daemonset partial podspec",
			daemonSet: &v1beta1.DaemonSetSpec{PodTemplate: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					ServiceAccountName: "sa-test",
				},
			}},
			wantDaemonSet: &beat.DaemonSetSpec{PodTemplate: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					ServiceAccountName: "sa-test",
				},
			}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			beat := v1beta1.Beat{
				Spec: v1beta1.BeatSpec{
					DaemonSet:  tt.daemonSet,
					Deployment: tt.deployment,
				},
			}
			got := newDriverParams(context.Background(), nil, beat)

			require.Equal(t, tt.wantDeployment, got.Deployment)
			require.Equal(t, tt.wantDaemonSet, got.DaemonSet)
		})
	}
}
