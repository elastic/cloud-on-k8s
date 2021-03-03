// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package expectations

import (
	"context"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
)

func TestClustersExpectation(t *testing.T) {
	client := k8s.NewFakeClient()
	e := NewClustersExpectations(client)

	cluster := types.NamespacedName{Namespace: "ns", Name: "name"}

	// requesting expectations for a particular cluster should create them on the fly
	clusterExp := e.ForCluster(cluster)
	satisfied, err := clusterExp.Satisfied()
	require.NoError(t, err)
	require.True(t, satisfied)

	// simulate a pod deletion expectation
	pod := corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "ns",
			Name:      "pod",
			UID:       uuid.NewUUID(),
		},
	}
	require.NoError(t, client.Create(context.Background(), &pod))
	clusterExp.ExpectDeletion(pod)
	satisfied, err = clusterExp.Satisfied()
	require.NoError(t, err)
	require.False(t, satisfied)

	// requesting expectations for that same cluster should return the same unsatisfied expectations
	clusterExp2 := e.ForCluster(cluster)
	satisfied, err = clusterExp2.Satisfied()
	require.NoError(t, err)
	require.False(t, satisfied)

	// requesting expectations for another cluster should be fine
	clusterExp = e.ForCluster(types.NamespacedName{Namespace: "ns", Name: "another-cluster"})
	satisfied, err = clusterExp.Satisfied()
	require.NoError(t, err)
	require.True(t, satisfied)

	// remove expectations for the first cluster
	e.RemoveCluster(cluster)
	// expectations should be recreated empty for that cluster
	clusterExp = e.ForCluster(cluster)
	satisfied, err = clusterExp.Satisfied()
	require.NoError(t, err)
	require.True(t, satisfied)
}
