// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodelabels

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestAnnotatePods(t *testing.T) {
	const namespace = "ns"
	podSelector := map[string]string{"app": "sample"}

	newPod := func(name, nodeName string, annotations map[string]string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Labels:      podSelector,
				Annotations: annotations,
			},
			Spec: corev1.PodSpec{NodeName: nodeName},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodScheduled, Status: corev1.ConditionTrue},
				},
			},
		}
	}

	node0 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "k8s-node-0",
		Labels: map[string]string{"topology.kubernetes.io/region": "europe-west1", "topology.kubernetes.io/zone": "europe-west1-a"},
	}}
	node1 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "k8s-node-1",
		Labels: map[string]string{"topology.kubernetes.io/region": "europe-west1", "topology.kubernetes.io/zone": "europe-west1-b"},
	}}

	tests := []struct {
		name           string
		expectedLabels []string
		objects        []client.Object
		wantErrMsg     string
		wantAnnots     map[string]map[string]string
	}{
		{
			name:           "no expected labels noop",
			expectedLabels: nil,
			objects: []client.Object{
				newPod("p0", "k8s-node-0", nil),
				node0,
			},
			wantAnnots: map[string]map[string]string{"p0": nil},
		},
		{
			name:           "node missing labels returns error",
			expectedLabels: []string{"topology.kubernetes.io/region"},
			objects: []client.Object{
				newPod("p0", "k8s-node-0", nil),
				&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "k8s-node-0"}},
			},
			wantErrMsg: "following annotations are expected to be set on Pod ns/p0 but do not exist as node labels: topology.kubernetes.io/region",
		},
		{
			name:           "annotates pods with node labels",
			expectedLabels: []string{"topology.kubernetes.io/region", "topology.kubernetes.io/zone"},
			objects: []client.Object{
				newPod("p0", "k8s-node-0", map[string]string{"existing": "value"}),
				newPod("p1", "k8s-node-1", nil),
				node0,
				node1,
			},
			wantAnnots: map[string]map[string]string{
				"p0": {
					"existing":                      "value",
					"topology.kubernetes.io/region": "europe-west1",
					"topology.kubernetes.io/zone":   "europe-west1-a",
				},
				"p1": {
					"topology.kubernetes.io/region": "europe-west1",
					"topology.kubernetes.io/zone":   "europe-west1-b",
				},
			},
		},
		{
			name:           "retains existing pod annotation value",
			expectedLabels: []string{"topology.kubernetes.io/region"},
			objects: []client.Object{
				newPod("p0", "k8s-node-0", map[string]string{"topology.kubernetes.io/region": "manual-value"}),
				node0,
			},
			wantAnnots: map[string]map[string]string{
				"p0": {"topology.kubernetes.io/region": "manual-value"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.objects...)
			results := AnnotatePods(context.Background(), c, namespace, podSelector, tt.expectedLabels, "sample")
			_, err := results.Aggregate()
			if tt.wantErrMsg != "" {
				assert.ErrorContains(t, err, tt.wantErrMsg)
				return
			}
			assert.NoError(t, err)
			pods := &corev1.PodList{}
			assert.NoError(t, c.List(context.Background(), pods))
			for _, pod := range pods.Items {
				wantAnnots, ok := tt.wantAnnots[pod.Name]
				if !ok {
					continue
				}
				for k, v := range wantAnnots {
					assert.Equal(t, v, pod.Annotations[k], "pod %s annotation %s", pod.Name, k)
				}
			}
		})
	}
}
