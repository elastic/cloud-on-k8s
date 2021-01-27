// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	"context"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestMarkPodAsUpdated(t *testing.T) {
	key := types.NamespacedName{
		Namespace: "ns1",
		Name:      "foo",
	}
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      "foo",
		},
	}
	type args struct {
		c   k8s.Client
		pod corev1.Pod
	}
	tests := []struct {
		name string
		args args
	}{
		{
			args: args{
				c:   k8s.NewFakeClient(pod.DeepCopy()),
				pod: pod,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			MarkPodAsUpdated(tt.args.c, tt.args.pod)
			// Ensure the label is present
			actualPod := &corev1.Pod{}
			assert.NoError(t, tt.args.c.Get(context.Background(), key, actualPod))
			assert.NotNil(t, actualPod.Annotations)
			previousValue, ok := actualPod.Annotations[UpdateAnnotation]
			assert.True(t, ok)
			// Trigger a new update
			MarkPodAsUpdated(tt.args.c, *actualPod)
			// Ensure the label is updated
			actualPod = &corev1.Pod{}
			assert.NoError(t, tt.args.c.Get(context.Background(), key, actualPod))
			assert.NotNil(t, actualPod.Annotations)
			newValue, ok := actualPod.Annotations[UpdateAnnotation]
			assert.True(t, ok)
			assert.True(t, newValue > previousValue)
		})
	}
}
