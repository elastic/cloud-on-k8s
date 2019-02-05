// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package label

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterFromResourceLabels(t *testing.T) {
	// test when label is not set
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
	}
	accessor, err := meta.Accessor(&pod)
	require.NoError(t, err)
	_, exists := ClusterFromResourceLabels(accessor)
	require.False(t, exists)

	// test when label is set
	pod.ObjectMeta.Labels = map[string]string{ClusterNameLabelName: "clusterName"}
	cluster, exists := ClusterFromResourceLabels(accessor)
	require.True(t, exists)
	require.Equal(t, types.NamespacedName{
		Namespace: "namespace",
		Name:      "clusterName",
	}, cluster)
}
