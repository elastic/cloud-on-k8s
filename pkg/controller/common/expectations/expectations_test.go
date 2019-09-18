// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package expectations

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGenerationsExpectations(t *testing.T) {
	expectations := NewExpectations()
	// check expectations that were not set
	obj := metav1.ObjectMeta{
		UID:        types.UID("abc"),
		Name:       "name",
		Namespace:  "namespace",
		Generation: 2,
	}
	require.True(t, expectations.SatisfiedGenerations(obj))
	// set expectations
	expectations.ExpectGeneration(obj)
	// check expectations are met for this object
	require.True(t, expectations.SatisfiedGenerations(obj))
	// but not for the same object with a smaller generation
	obj.Generation = 1
	require.False(t, expectations.SatisfiedGenerations(obj))
	// a different object (different UID) should have expectations met
	obj.UID = types.UID("another")
	require.True(t, expectations.SatisfiedGenerations(obj))
}

func newPod(clusterName types.NamespacedName, podName string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: clusterName.Namespace,
			Name:      podName,
			UID:       uuid.NewUUID(),
			Labels:    label.NewLabels(clusterName),
		},
	}
}

func TestExpectations_ExpectDeletion(t *testing.T) {
	testCluster1 := types.NamespacedName{
		Namespace: "ns1",
		Name:      "cluster1",
	}
	testCluster2 := types.NamespacedName{
		Namespace: "ns2",
		Name:      "cluster2",
	}
	testCluster3 := types.NamespacedName{
		Namespace: "ns2",
		Name:      "cluster3",
	}

	e := NewExpectations()
	// Initial state
	assert.Equal(t, len(e.deletions[testCluster1]), 0)
	assert.Equal(t, len(e.deletions[testCluster2]), 0)

	cluster1Pod1 := newPod(testCluster1, "cluster1Pod1")
	e.ExpectDeletion(cluster1Pod1)
	assert.Equal(t, len(e.deletions[testCluster1]), 1)
	assert.Equal(t, len(e.deletions[testCluster2]), 0)

	cluster2Pod1 := newPod(testCluster2, "cluster2Pod1")
	e.ExpectDeletion(cluster2Pod1)
	assert.Equal(t, len(e.deletions[testCluster1]), 1)
	assert.Equal(t, len(e.deletions[testCluster2]), 1)

	cluster1Pod2 := newPod(testCluster1, "cluster1Pod2")
	e.ExpectDeletion(cluster1Pod2)
	assert.Equal(t, len(e.deletions[testCluster1]), 2)
	assert.Equal(t, len(e.deletions[testCluster2]), 1)

	cluster1Pod3 := newPod(testCluster1, "cluster1Pod3")
	e.ExpectDeletion(cluster1Pod3)
	assert.Equal(t, len(e.deletions[testCluster1]), 3)
	assert.Equal(t, len(e.deletions[testCluster2]), 1)

	e.CancelExpectedDeletion(cluster1Pod1)
	assert.Equal(t, len(e.deletions[testCluster1]), 2)
	assert.Equal(t, len(e.deletions[testCluster2]), 1)

	e.CancelExpectedDeletion(cluster1Pod2)
	assert.Equal(t, len(e.deletions[testCluster1]), 1)
	assert.Equal(t, len(e.deletions[testCluster2]), 1)

	cluster3Pod1 := newPod(testCluster1, "cluster3Pod1")
	e.CancelExpectedDeletion(cluster3Pod1)
	assert.Equal(t, len(e.deletions[testCluster1]), 1)
	assert.Equal(t, len(e.deletions[testCluster2]), 1)
	assert.Equal(t, len(e.deletions[testCluster3]), 0)

	// Create a fake client with new UID for the last remaining Pod in cluster1
	cluster1Pod3 = newPod(testCluster1, "cluster1Pod3")
	client := k8s.WrapClient(fake.NewFakeClient(&cluster1Pod3, &cluster2Pod1))

	// UID has changed for the last Pod of cluster1, expectation must be satisfied
	satisfied, err := e.SatisfiedDeletions(client, testCluster1)
	require.NoError(t, err)
	assert.Equal(t, satisfied, true)
	cluster1deletions := e.deletions[testCluster1]
	assert.Equal(t, len(cluster1deletions), 0)

	// We still have a remaining Pod for cluster2
	satisfied, err = e.SatisfiedDeletions(client, testCluster2)
	require.NoError(t, err)
	assert.Equal(t, satisfied, false)
}
