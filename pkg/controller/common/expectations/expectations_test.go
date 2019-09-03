// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package expectations

import (
	"testing"

	"github.com/magiconair/properties/assert"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	require.True(t, expectations.GenerationExpected(obj))
	// set expectations
	expectations.ExpectGeneration(obj)
	// check expectations are met for this object
	require.True(t, expectations.GenerationExpected(obj))
	// but not for the same object with a smaller generation
	obj.Generation = 1
	require.False(t, expectations.GenerationExpected(obj))
	// a different object (different UID) should have expectations met
	obj.UID = types.UID("another")
	require.True(t, expectations.GenerationExpected(obj))
}

func newPod(clusterName types.NamespacedName, podName string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: clusterName.Namespace,
			Name:      podName,
			Labels:    label.NewLabels(clusterName),
		},
	}
}

type cluster1DeletionChecker struct{}

func (c cluster1DeletionChecker) CanRemoveExpectation(meta metav1.ObjectMeta) (bool, error) {
	if meta.Namespace == "ns1" {
		return true, nil
	}
	return false, nil
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

	e.ExpectDeletion(newPod(testCluster1, "pod1_1"))
	assert.Equal(t, len(e.deletions[testCluster1]), 1)
	assert.Equal(t, len(e.deletions[testCluster2]), 0)

	e.ExpectDeletion(newPod(testCluster2, "pod2_1"))
	assert.Equal(t, len(e.deletions[testCluster1]), 1)
	assert.Equal(t, len(e.deletions[testCluster2]), 1)

	e.ExpectDeletion(newPod(testCluster1, "pod1_2"))
	assert.Equal(t, len(e.deletions[testCluster1]), 2)
	assert.Equal(t, len(e.deletions[testCluster2]), 1)

	e.ExpectDeletion(newPod(testCluster1, "pod1_3"))
	assert.Equal(t, len(e.deletions[testCluster1]), 3)
	assert.Equal(t, len(e.deletions[testCluster2]), 1)

	e.CancelExpectedDeletion(newPod(testCluster1, "pod1_1"))
	assert.Equal(t, len(e.deletions[testCluster1]), 2)
	assert.Equal(t, len(e.deletions[testCluster2]), 1)

	e.CancelExpectedDeletion(newPod(testCluster1, "pod1_2"))
	assert.Equal(t, len(e.deletions[testCluster1]), 1)
	assert.Equal(t, len(e.deletions[testCluster2]), 1)

	e.CancelExpectedDeletion(newPod(testCluster3, "pod3_1"))
	assert.Equal(t, len(e.deletions[testCluster1]), 1)
	assert.Equal(t, len(e.deletions[testCluster2]), 1)
	assert.Equal(t, len(e.deletions[testCluster3]), 0)

	checker := cluster1DeletionChecker{}
	satisfied, err := e.SatisfiedDeletions(testCluster1, checker)
	require.NoError(t, err)
	assert.Equal(t, satisfied, true)
	satisfied, err = e.SatisfiedDeletions(testCluster2, checker)
	require.NoError(t, err)
	assert.Equal(t, satisfied, false)
}
