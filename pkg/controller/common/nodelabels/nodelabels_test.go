// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodelabels

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// fakeTarget is a test implementation of AnnotationTarget.
type fakeTarget struct {
	*corev1.Pod // embeds metav1.Object via ObjectMeta
	labels      []string
	selector    map[string]string
}

func (f *fakeTarget) DownwardNodeLabels() []string         { return f.labels }
func (f *fakeTarget) GetIdentityLabels() map[string]string { return f.selector }

func TestMaybeAddWaitForAnnotationsInitContainer(t *testing.T) {
	const operatorImage = "docker.elastic.co/eck/eck-operator:test"

	newTarget := func(labels ...string) *fakeTarget {
		return &fakeTarget{
			Pod:    &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}},
			labels: labels,
		}
	}
	newBuilder := func() *defaults.PodTemplateBuilder {
		return defaults.NewPodTemplateBuilder(corev1.PodTemplateSpec{}, "main")
	}

	tests := []struct {
		name          string
		builder       *defaults.PodTemplateBuilder
		target        *fakeTarget
		operatorImage string
		wantErr       bool
		assertions    func(t *testing.T, got *defaults.PodTemplateBuilder)
	}{
		{
			name:          "no downward node labels: no-op",
			builder:       newBuilder(),
			target:        newTarget(),
			operatorImage: operatorImage,
			assertions: func(t *testing.T, got *defaults.PodTemplateBuilder) {
				t.Helper()
				assert.Empty(t, got.PodTemplate.Spec.InitContainers)
				assert.Empty(t, got.PodTemplate.Spec.Volumes)
			},
		},
		{
			name:          "labels set but empty operator image: error",
			builder:       newBuilder(),
			target:        newTarget("topology.kubernetes.io/zone"),
			operatorImage: "",
			wantErr:       true,
		},
		{
			name:          "single annotation: init container, volume, and main container mount added",
			builder:       newBuilder(),
			target:        newTarget("topology.kubernetes.io/zone"),
			operatorImage: operatorImage,
			assertions: func(t *testing.T, got *defaults.PodTemplateBuilder) {
				t.Helper()
				require.Len(t, got.PodTemplate.Spec.InitContainers, 1)
				ic := got.PodTemplate.Spec.InitContainers[0]
				assert.Equal(t, "elastic-internal-wait-for-node-labels", ic.Name)
				assert.Equal(t, operatorImage, ic.Image)
				require.GreaterOrEqual(t, len(ic.Command), 2)
				assert.Equal(t, "/elastic-operator", ic.Command[0])
				assert.Equal(t, "wait-for-annotations", ic.Command[1])
				assert.Contains(t, ic.Command, "--annotation=topology.kubernetes.io/zone")
				assert.NotEmpty(t, got.PodTemplate.Spec.Volumes)
				// main container should also have the annotations file mounted so the
				// running application can read its own topology annotations.
				var hasMount bool
				for _, vm := range got.PodTemplate.Spec.Containers[0].VolumeMounts {
					if vm.MountPath == "/mnt/elastic-internal/downward-api" {
						hasMount = true
						break
					}
				}
				assert.True(t, hasMount, "main container should have the downward-api volume mounted")
			},
		},
		{
			name:          "multiple annotations: one --annotation flag per label",
			builder:       newBuilder(),
			target:        newTarget("topology.kubernetes.io/zone", "topology.kubernetes.io/region"),
			operatorImage: operatorImage,
			assertions: func(t *testing.T, got *defaults.PodTemplateBuilder) {
				t.Helper()
				require.Len(t, got.PodTemplate.Spec.InitContainers, 1)
				cmd := got.PodTemplate.Spec.InitContainers[0].Command
				assert.Contains(t, cmd, "--annotation=topology.kubernetes.io/zone")
				assert.Contains(t, cmd, "--annotation=topology.kubernetes.io/region")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MaybeAddWaitForAnnotationsInitContainer(tt.builder, tt.target, tt.operatorImage)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.assertions(t, got)
		})
	}
}

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
			target := &fakeTarget{
				Pod:      &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "sample"}},
				labels:   tt.expectedLabels,
				selector: podSelector,
			}
			results := AnnotatePods(context.Background(), c, target)
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
