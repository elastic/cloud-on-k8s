// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func Test_defaultDriver_maybeForceUpgradePods(t *testing.T) {
	tests := []struct {
		name              string
		actualPods        []corev1.Pod
		podsToUpgrade     []corev1.Pod
		wantAttempted     bool
		wantRemainingPods []corev1.Pod
	}{
		{
			name: "no pods to upgrade",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetB", Ready: true, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: nil,
			wantAttempted: false,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetB", Ready: true, ResourceVersion: "999"}.Build(),
			},
		},
		{
			name: "pods bootlooping in all StatefulSets, upgrade them all",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
			wantAttempted:     true,
			wantRemainingPods: nil,
		},
		{
			name: "all pods bootlooping in StatefulSet B, upgrade them",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Ready: false, RestartCount: 0, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Ready: false, RestartCount: 0, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
			wantAttempted: true,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Ready: false, RestartCount: 0, ResourceVersion: "999"}.Build(),
			},
		},
		{
			name: "all pods pending in all StatefulSets, upgrade them all",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
			wantAttempted:     true,
			wantRemainingPods: nil,
		},
		{
			name: "all pods pending in StatefulSet A, upgrade them",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Phase: corev1.PodRunning, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Phase: corev1.PodRunning, ResourceVersion: "999"}.Build(),
			},
			wantAttempted: true,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Phase: corev1.PodRunning, ResourceVersion: "999"}.Build(),
			},
		},
		{
			name: "1/2 pod to upgrade",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
			wantAttempted: true,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
		},
		{
			name: "at least one pod ready, don't upgrade any",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Ready: true, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod3", StatefulSetName: "ssetA", Ready: false, Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Ready: true, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod3", StatefulSetName: "ssetA", Ready: false, Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
			wantAttempted: false,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Ready: true, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod3", StatefulSetName: "ssetA", Ready: false, Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
		},
		{
			name: "at least one pod not bootlooping, don't restart any",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Ready: false, RestartCount: 0, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod3", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Ready: false, RestartCount: 0, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod3", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
			wantAttempted: false,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Ready: false, RestartCount: 0, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod3", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeObjs := make([]runtime.Object, 0, len(tt.actualPods))
			for i := range tt.actualPods {
				runtimeObjs = append(runtimeObjs, &tt.actualPods[i])
			}
			k8sClient := k8s.NewFakeClient(runtimeObjs...)
			d := &defaultDriver{
				DefaultDriverParameters: DefaultDriverParameters{
					Client:         k8sClient,
					Expectations:   expectations.NewExpectations(k8sClient),
					ReconcileState: reconcile.MustNewState(esv1.Elasticsearch{}),
				},
			}

			attempted, err := d.maybeForceUpgradePods(tt.actualPods, tt.podsToUpgrade)
			require.NoError(t, err)
			require.Equal(t, tt.wantAttempted, attempted)
			var pods corev1.PodList
			require.NoError(t, k8sClient.List(context.Background(), &pods))
			require.ElementsMatch(t, tt.wantRemainingPods, pods.Items)
		})
	}
}
