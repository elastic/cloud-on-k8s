// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
)

// expectedPodsAnnotations holds pod -> expectedAnnotation -> expectedValue
type expectedPodsAnnotations map[string]map[string]string

func (e expectedPodsAnnotations) isEmpty() bool {
	return len(e) == 0
}

func (e expectedPodsAnnotations) assertPodsMatch(t *testing.T, pods []corev1.Pod) {
	t.Helper()
	assert.Equal(t, len(e), len(pods), "expected %d Pods with annotations, got %d Pods", len(e), len(pods))
	for _, pod := range pods {
		expectedAnnotations, exists := e[pod.Name]
		assert.True(t, exists)
		for expectedAnnotation, expectedValue := range expectedAnnotations {
			actualValue, exists := pod.Annotations[expectedAnnotation]
			assert.True(t, exists, "Pod %s: expected annotation %s", pod.Name, expectedAnnotation)
			assert.Equal(
				t,
				expectedValue,
				actualValue,
				"Pod %s: expected value \"%s\" for annotation \"%s\", got value \"%s\"",
				pod.Name, expectedValue, expectedAnnotation, actualValue,
			)
		}
	}
}

const esName = "elasticsearch-sample"

func Test_annotatePodsWithNodeLabels(t *testing.T) {
	type args struct {
		ctx     context.Context
		es      *esv1.Elasticsearch
		objects []runtime.Object
	}
	tests := []struct {
		name                string
		args                args
		wantErrMsg          string
		expectedAnnotations expectedPodsAnnotations
	}{
		{
			name: "No annotations on K8S nodes",
			args: args{
				es: &esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:        esName,
						Namespace:   "ns",
						Annotations: map[string]string{"eck.k8s.elastic.co/downward-node-labels": "topology.kubernetes.io/region,topology.kubernetes.io/zone"},
					},
				},
				objects: []runtime.Object{
					newPodBuilder("elasticsearch-sample-es-default-0").scheduledOn("k8s-node-0").build(),
					newPodBuilder("elasticsearch-sample-es-default-1").scheduledOn("k8s-node-1").build(),
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "k8s-node-0"}},
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "k8s-node-1"}},
				},
				ctx: context.Background(),
			},
			wantErrMsg: "following annotations are expected to be set on Pod ns/elasticsearch-sample-es-default-0 but do not exist as node labels: topology.kubernetes.io/region,topology.kubernetes.io/zone",
		},
		{
			name: "No initial annotations on the Pods",
			args: args{
				es: &esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:        esName,
						Namespace:   "ns",
						Annotations: map[string]string{"eck.k8s.elastic.co/downward-node-labels": "topology.kubernetes.io/region,topology.kubernetes.io/zone"},
					},
				},
				objects: []runtime.Object{
					newPodBuilder("elasticsearch-sample-es-default-0").scheduledOn("k8s-node-0").build(),
					newPodBuilder("elasticsearch-sample-es-default-1").scheduledOn("k8s-node-1").build(),
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "k8s-node-0", Labels: map[string]string{"topology.kubernetes.io/region": "europe-west1", "topology.kubernetes.io/zone": "europe-west1-a"}}},
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "k8s-node-1", Labels: map[string]string{"topology.kubernetes.io/region": "europe-west1", "topology.kubernetes.io/zone": "europe-west1-b"}}},
				},
				ctx: context.Background(),
			},
			expectedAnnotations: expectedPodsAnnotations{
				"elasticsearch-sample-es-default-0": {
					"topology.kubernetes.io/region": "europe-west1",
					"topology.kubernetes.io/zone":   "europe-west1-a",
				},
				"elasticsearch-sample-es-default-1": {
					"topology.kubernetes.io/region": "europe-west1",
					"topology.kubernetes.io/zone":   "europe-west1-b",
				},
			},
		},
		{
			name: "With initial annotations on the Pods",
			args: args{
				es: &esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:        esName,
						Namespace:   "ns",
						Annotations: map[string]string{"eck.k8s.elastic.co/downward-node-labels": "topology.kubernetes.io/region,topology.kubernetes.io/zone"},
					},
				},
				objects: []runtime.Object{
					newPodBuilder("elasticsearch-sample-es-default-0").scheduledOn("k8s-node-0").withAnnotation(map[string]string{"foo": "bar"}).build(),
					newPodBuilder("elasticsearch-sample-es-default-1").scheduledOn("k8s-node-1").withAnnotation(map[string]string{"foo": "bar"}).build(),
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "k8s-node-0", Labels: map[string]string{"topology.kubernetes.io/region": "europe-west1", "topology.kubernetes.io/zone": "europe-west1-a"}}},
					&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "k8s-node-1", Labels: map[string]string{"topology.kubernetes.io/region": "europe-west1", "topology.kubernetes.io/zone": "europe-west1-b"}}},
				},
				ctx: context.Background(),
			},
			expectedAnnotations: expectedPodsAnnotations{
				"elasticsearch-sample-es-default-0": {
					"topology.kubernetes.io/region": "europe-west1",
					"topology.kubernetes.io/zone":   "europe-west1-a",
					"foo":                           "bar",
				},
				"elasticsearch-sample-es-default-1": {
					"topology.kubernetes.io/region": "europe-west1",
					"topology.kubernetes.io/zone":   "europe-west1-b",
					"foo":                           "bar",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allObjects := append(tt.args.objects, tt.args.es)
			k8sClient := k8s.NewFakeClient(allObjects...)
			got := annotatePodsWithNodeLabels(tt.args.ctx, k8sClient, *tt.args.es)
			_, err := got.Aggregate()
			if tt.wantErrMsg != "" {
				assert.Containsf(t, err.Error(), tt.wantErrMsg, "expected error containing %q, got %s", tt.wantErrMsg, err)
			} else {
				assert.NoError(t, err)
			}
			if !tt.expectedAnnotations.isEmpty() {
				podList := &corev1.PodList{}
				assert.NoError(t, k8sClient.List(context.Background(), podList))
				tt.expectedAnnotations.assertPodsMatch(t, podList.Items)
			}
		})
	}
}

type podBuilder struct {
	podName, nodeName string
	scheduled         bool
	annotations       map[string]string
}

func newPodBuilder(podName string) *podBuilder {
	return &podBuilder{
		podName: podName,
	}
}

func (pb *podBuilder) scheduledOn(nodeName string) *podBuilder {
	pb.scheduled = true
	pb.nodeName = nodeName
	return pb
}

func (pb *podBuilder) withAnnotation(annotations map[string]string) *podBuilder {
	pb.annotations = annotations
	return pb
}

func (pb *podBuilder) build() *corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: pb.podName, Namespace: "ns",
			Labels:      map[string]string{"elasticsearch.k8s.elastic.co/cluster-name": esName},
			Annotations: pb.annotations,
		},
		Spec: corev1.PodSpec{
			NodeName: pb.nodeName,
		},
	}
	if pb.scheduled {
		pod.Status = corev1.PodStatus{
			Phase: "",
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionTrue,
				},
			},
		}
	}
	return &pod
}
