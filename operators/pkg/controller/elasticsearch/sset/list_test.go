// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

var ssetv7 = appsv1.StatefulSet{
	Spec: appsv1.StatefulSetSpec{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					label.VersionLabelName: "7.1.0",
				},
			},
		},
	},
}

func TestESVersionMatch(t *testing.T) {
	require.Equal(t, true,
		ESVersionMatch(ssetv7, func(v version.Version) bool {
			return v.Major == 7
		}),
	)
	require.Equal(t, false,
		ESVersionMatch(ssetv7, func(v version.Version) bool {
			return v.Major == 6
		}),
	)
}

func TestAtLeastOneESVersionMatch(t *testing.T) {
	ssetv6 := *ssetv7.DeepCopy()
	ssetv6.Spec.Template.Labels[label.VersionLabelName] = "6.8.0"

	require.Equal(t, true,
		AtLeastOneESVersionMatch(StatefulSetList{ssetv6, ssetv7}, func(v version.Version) bool {
			return v.Major == 7
		}),
	)
	require.Equal(t, false,
		AtLeastOneESVersionMatch(StatefulSetList{ssetv6, ssetv6}, func(v version.Version) bool {
			return v.Major == 7
		}),
	)
}

func TestStatefulSetList_GetExistingPods(t *testing.T) {
	// 2 pods that belong to the sset
	pod1 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod1",
			Labels: map[string]string{
				label.StatefulSetNameLabelName: ssetv7.Name,
			},
		},
	}
	pod2 := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod2",
			Labels: map[string]string{
				label.StatefulSetNameLabelName: ssetv7.Name,
			},
		},
	}
	client := k8s.WrapClient(fake.NewFakeClient(&pod1, &pod2))
	pods, err := StatefulSetList{ssetv7}.GetActualPods(client)
	require.NoError(t, err)
	require.Equal(t, []corev1.Pod{pod1, pod2}, pods)
	// TODO: test with an additional pod that does not belong to the sset and
	//  check it is not returned.
	//  This cannot be done currently since the fake client does not support label list options.
}
