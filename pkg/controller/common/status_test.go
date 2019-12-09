// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
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
			client := k8s.WrappedFakeClient(&initialPod)
			err := workaroundStatusUpdateError(tt.err, client, &updatedPod)
			require.Equal(t, tt.wantErr, err)
			// get back the pod to check if it was updated
			var pod corev1.Pod
			require.NoError(t, client.Get(k8s.ExtractNamespacedName(&initialPod), &pod))
			require.Equal(t, tt.wantUpdate, pod.Status.Message == "updated")
		})
	}
}
