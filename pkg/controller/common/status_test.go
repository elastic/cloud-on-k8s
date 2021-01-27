// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"context"
	"errors"
	"reflect"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func Test_workaroundStatusUpdateError(t *testing.T) {
	initialPod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"}}
	updatedPod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "name"}, Status: corev1.PodStatus{Message: "updated"}}
	tests := []struct {
		name       string
		err        error
		wantErr    error
		wantUpdate bool
	}{
		{
			name:       "no error",
			err:        nil,
			wantErr:    nil,
			wantUpdate: false,
		},
		{
			name:       "different error",
			err:        errors.New("something else"),
			wantErr:    errors.New("something else"),
			wantUpdate: false,
		},
		{
			name:       "validation error",
			err:        apierrors.NewInvalid(initialPod.GroupVersionKind().GroupKind(), initialPod.Name, field.ErrorList{}),
			wantErr:    nil,
			wantUpdate: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.NewFakeClient(&initialPod)
			err := workaroundStatusUpdateError(tt.err, client, &updatedPod)
			require.Equal(t, tt.wantErr, err)
			// get back the pod to check if it was updated
			var pod corev1.Pod
			require.NoError(t, client.Get(context.Background(), k8s.ExtractNamespacedName(&initialPod), &pod))
			require.Equal(t, tt.wantUpdate, pod.Status.Message == "updated")
		})
	}
}

func TestLowestVersionFromPods(t *testing.T) {
	versionLabel := "version-label"
	type args struct {
		currentVersion string
		pods           []corev1.Pod
		versionLabel   string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "all pods have the same version: return it",
			args: args{
				pods: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{versionLabel: "7.7.0"}}},
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{versionLabel: "7.7.0"}}},
				},
				currentVersion: "",
				versionLabel:   versionLabel,
			},
			want: "7.7.0",
		},
		{
			name: "return the lowest running version",
			args: args{
				pods: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{versionLabel: "7.7.0"}}},
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{versionLabel: "7.6.0"}}},
				},
				currentVersion: "",
				versionLabel:   versionLabel,
			},
			want: "7.6.0",
		},
		{
			name: "cannot parse version from pods: return the current version",
			args: args{
				pods: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{versionLabel: "invalid"}}},
				},
				currentVersion: "7.7.0",
				versionLabel:   versionLabel,
			},
			want: "7.7.0",
		},
		{
			name: "no pods: return the current version",
			args: args{
				pods:           []corev1.Pod{},
				currentVersion: "7.7.0",
				versionLabel:   versionLabel,
			},
			want: "7.7.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LowestVersionFromPods(tt.args.currentVersion, tt.args.pods, tt.args.versionLabel); got != tt.want {
				t.Errorf("LowestVersionFromPods() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeploymentStatus(t *testing.T) {
	type args struct {
		current      commonv1.DeploymentStatus
		dep          appsv1.Deployment
		pods         []corev1.Pod
		versionLabel string
	}
	tests := []struct {
		name string
		args args
		want commonv1.DeploymentStatus
	}{
		{
			name: "happy path: set all status fields",
			args: args{
				current: commonv1.DeploymentStatus{},
				dep: appsv1.Deployment{
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 3,
						Conditions: []appsv1.DeploymentCondition{
							{
								Type:   appsv1.DeploymentAvailable,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				pods: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.0"}}},
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.0"}}},
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.0"}}},
				},
				versionLabel: "version-label",
			},
			want: commonv1.DeploymentStatus{
				AvailableNodes: 3,
				Version:        "7.7.0",
				Health:         commonv1.GreenHealth,
			},
		},
		{
			name: "red health",
			args: args{
				current: commonv1.DeploymentStatus{},
				dep: appsv1.Deployment{
					Status: appsv1.DeploymentStatus{
						AvailableReplicas: 3,
						Conditions: []appsv1.DeploymentCondition{
							{
								Type:   appsv1.DeploymentAvailable,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
				pods: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.0"}}},
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.0"}}},
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.0"}}},
				},
				versionLabel: "version-label",
			},
			want: commonv1.DeploymentStatus{
				AvailableNodes: 3,
				Version:        "7.7.0",
				Health:         commonv1.RedHealth,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeploymentStatus(tt.args.current, tt.args.dep, tt.args.pods, tt.args.versionLabel); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DeploymentStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}
