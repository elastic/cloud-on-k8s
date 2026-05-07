// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package expectations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestClustersExpectation(t *testing.T) {
	client := k8s.NewFakeClient()
	e := NewClustersExpectations(client, &appsv1.StatefulSet{})

	cluster := types.NamespacedName{Namespace: "ns", Name: "name"}

	// requesting expectations for a particular cluster should create them on the fly
	clusterExp := e.ForCluster(cluster)
	satisfied, reason, err := clusterExp.Satisfied()
	require.NoError(t, err)
	require.True(t, satisfied)
	require.Equal(t, "", reason)

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
	satisfied, reason, err = clusterExp.Satisfied()
	require.NoError(t, err)
	require.False(t, satisfied)
	require.NotEqual(t, "", reason)

	// requesting expectations for that same cluster should return the same unsatisfied expectations
	clusterExp2 := e.ForCluster(cluster)
	satisfied, reason, err = clusterExp2.Satisfied()
	require.NoError(t, err)
	require.False(t, satisfied)
	require.NotEqual(t, "", reason)

	// requesting expectations for another cluster should be fine
	clusterExp = e.ForCluster(types.NamespacedName{Namespace: "ns", Name: "another-cluster"})
	satisfied, reason, err = clusterExp.Satisfied()
	require.NoError(t, err)
	require.True(t, satisfied)
	require.Equal(t, "", reason)

	// remove expectations for the first cluster
	e.RemoveCluster(cluster)
	// expectations should be recreated empty for that cluster
	clusterExp = e.ForCluster(cluster)
	satisfied, reason, err = clusterExp.Satisfied()
	require.NoError(t, err)
	require.True(t, satisfied)
	require.Equal(t, "", reason)
}

func TestClustersExpectation_ConcurrentForCluster(t *testing.T) {
	client := k8s.NewFakeClient()
	e := NewClustersExpectations(client, &appsv1.StatefulSet{})

	cluster := types.NamespacedName{Namespace: "ns", Name: "name"}

	// Create a pod to use for deletion expectations
	pod := corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "ns",
			Name:      "pod",
			UID:       uuid.NewUUID(),
		},
	}
	require.NoError(t, client.Create(context.Background(), &pod))

	// Simulate the race condition scenario:
	// 1. Goroutine 1: calls get(), gets nil (not found)
	// 2. Goroutine 2: calls get(), gets nil (not found)
	// 3. Goroutine 1: calls create(), creates and stores expectation A
	// 4. Goroutine 2: calls create(), should return expectation A (not create B)

	// Both "goroutines" call get() first, both get nil
	exp1, ok1 := e.get(cluster)
	exp2, ok2 := e.get(cluster)
	require.False(t, ok1)
	require.False(t, ok2)
	require.Nil(t, exp1)
	require.Nil(t, exp2)

	// "Goroutine 1" calls create() first
	exp1 = e.create(cluster)
	// "Goroutine 2" calls create() second - should return same instance due to double-checked locking
	exp2 = e.create(cluster)

	// Both should have received the same Expectations instance.
	// Without the double-checked locking fix, exp2 would be a new instance
	// that overwrites exp1's instance in the map.
	require.Same(t, exp1, exp2,
		"both callers should receive the same Expectations instance")

	// Verify that modifications are visible to both
	exp1.ExpectDeletion(pod)
	satisfied, _, err := exp2.Satisfied()
	require.NoError(t, err)
	require.False(t, satisfied, "deletion expectation should be visible from both references")
}
