// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package annotation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
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
			MarkPodAsUpdated(context.Background(), tt.args.c, tt.args.pod)
			// Ensure the label is present
			actualPod := &corev1.Pod{}
			assert.NoError(t, tt.args.c.Get(context.Background(), key, actualPod))
			assert.NotNil(t, actualPod.Annotations)
			previousValue, ok := actualPod.Annotations[UpdateAnnotation]
			assert.True(t, ok)
			// Trigger a new update
			MarkPodAsUpdated(context.Background(), tt.args.c, *actualPod)
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
