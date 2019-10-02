// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"

	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
)

func Test_defaultDriver_maybeForceUpgrade(t *testing.T) {
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
				sset.TestPod{Name: "pod1", Ready: false}.Build(),
				sset.TestPod{Name: "pod2", Ready: true}.Build(),
			},
			podsToUpgrade: nil,
			wantAttempted: false,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", Ready: false}.Build(),
				sset.TestPod{Name: "pod2", Ready: true}.Build(),
			},
		},
		{
			name: "pods bootlooping, upgrade them all",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", Ready: false, RestartCount: 1}.Build(),
				sset.TestPod{Name: "pod2", Ready: false, RestartCount: 1}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "pod1", Ready: false, RestartCount: 1}.Build(),
				sset.TestPod{Name: "pod2", Ready: false, RestartCount: 1}.Build(),
			},
			wantAttempted:     true,
			wantRemainingPods: nil,
		},
		{
			name: "pods pending, upgrade them all",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", Phase: corev1.PodPending}.Build(),
				sset.TestPod{Name: "pod2", Phase: corev1.PodPending}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "pod1", Phase: corev1.PodPending}.Build(),
				sset.TestPod{Name: "pod2", Phase: corev1.PodPending}.Build(),
			},
			wantAttempted:     true,
			wantRemainingPods: nil,
		},
		{
			name: "1/2 pod to upgrade",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", Phase: corev1.PodPending}.Build(),
				sset.TestPod{Name: "pod2", Phase: corev1.PodPending}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "pod2", Phase: corev1.PodPending}.Build(),
			},
			wantAttempted: true,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", Phase: corev1.PodPending}.Build(),
			},
		},
		{
			name: "at least one pod ready, don't upgrade any",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", Ready: false, Phase: corev1.PodPending}.Build(),
				sset.TestPod{Name: "pod2", Ready: true}.Build(),
				sset.TestPod{Name: "pod3", Ready: false, Phase: corev1.PodPending}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "pod1", Ready: false, Phase: corev1.PodPending}.Build(),
				sset.TestPod{Name: "pod2", Ready: true}.Build(),
				sset.TestPod{Name: "pod3", Ready: false, Phase: corev1.PodPending}.Build(),
			},
			wantAttempted: false,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", Ready: false, Phase: corev1.PodPending}.Build(),
				sset.TestPod{Name: "pod2", Ready: true}.Build(),
				sset.TestPod{Name: "pod3", Ready: false, Phase: corev1.PodPending}.Build(),
			},
		},
		{
			name: "at least one pod not bootlooping, don't restart any",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", Ready: false, RestartCount: 1}.Build(),
				sset.TestPod{Name: "pod2", Ready: false, RestartCount: 0}.Build(),
				sset.TestPod{Name: "pod3", Ready: false, RestartCount: 1}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "pod1", Ready: false, RestartCount: 1}.Build(),
				sset.TestPod{Name: "pod2", Ready: false, RestartCount: 0}.Build(),
				sset.TestPod{Name: "pod3", Ready: false, RestartCount: 1}.Build(),
			},
			wantAttempted: false,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", Ready: false, RestartCount: 1}.Build(),
				sset.TestPod{Name: "pod2", Ready: false, RestartCount: 0}.Build(),
				sset.TestPod{Name: "pod3", Ready: false, RestartCount: 1}.Build(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeObjs := make([]runtime.Object, 0, len(tt.actualPods))
			for i := range tt.actualPods {
				runtimeObjs = append(runtimeObjs, &tt.actualPods[i])
			}
			k8sClient := k8s.WrapClient(fake.NewFakeClient(runtimeObjs...))
			d := &defaultDriver{
				DefaultDriverParameters: DefaultDriverParameters{
					Client:       k8sClient,
					Expectations: expectations.NewExpectations(),
				},
			}

			attempted, err := d.maybeForceUpgrade(tt.actualPods, tt.podsToUpgrade)
			require.NoError(t, err)
			require.Equal(t, tt.wantAttempted, attempted)
			var pods corev1.PodList
			require.NoError(t, k8sClient.List(&pods))
			require.ElementsMatch(t, tt.wantRemainingPods, pods.Items)
		})
	}
}
