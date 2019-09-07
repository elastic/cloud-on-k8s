// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package expectations

import (
	"errors"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("expectations")
)

type deleteExpectations map[types.NamespacedName]map[types.NamespacedName]types.UID

// Note: expectations are NOT thread-safe.
// TODO: garbage collect/finalize deprecated expectation
type Expectations struct {
	// StatefulSet -> generation
	generations map[types.UID]int64
	// Cluster -> Pod -> UID
	deletions deleteExpectations
}

func NewExpectations() *Expectations {
	return &Expectations{
		generations: make(map[types.UID]int64),
		deletions:   make(deleteExpectations),
	}
}

// -- Deletions expectations

// ExpectDeletion registers an expected deletion for the given Pod.
func (e *Expectations) ExpectDeletion(pod v1.Pod) {
	cluster, exists := getClusterFromPodLabel(pod)
	if !exists {
		return // Should not happen as all Pods should have the correct labels
	}
	var expectedPods map[types.NamespacedName]types.UID
	expectedPods, exists = e.deletions[cluster]
	if !exists {
		expectedPods = map[types.NamespacedName]types.UID{}
		e.deletions[cluster] = expectedPods
	}
	expectedPods[k8s.ExtractNamespacedName(&pod)] = pod.UID
}

// CancelExpectedDeletion removes an expected deletion for the given Pod.
func (e *Expectations) CancelExpectedDeletion(pod v1.Pod) {
	cluster, exists := getClusterFromPodLabel(pod)
	if !exists {
		return // Should not happen as all Pods should have the correct labels
	}
	var expectedPods map[types.NamespacedName]types.UID
	expectedPods, exists = e.deletions[cluster]
	if !exists {
		return
	}
	delete(expectedPods, k8s.ExtractNamespacedName(&pod))
}

func getClusterFromPodLabel(pod v1.Pod) (types.NamespacedName, bool) {
	cluster, exists := label.ClusterFromResourceLabels(pod.GetObjectMeta())
	if !exists {
		log.Error(errors.New("cannot find the cluster label on Pod"),
			"Failed to get cluster from Pod annotation",
			"pod_name", pod.Name,
			"pod_namespace", pod.Namespace)
	}
	return cluster, exists
}

// DeletionChecker is used to check if a Pod can be remove from the deletions expectations.
type DeletionChecker interface {
	CanRemoveExpectation(podName types.NamespacedName, uid types.UID) (bool, error)
}

// SatisfiedDeletions uses the provided DeletionChecker to check if the delete expectations are satisfied.
func (e *Expectations) SatisfiedDeletions(cluster types.NamespacedName, checker DeletionChecker) (bool, error) {
	// Get all the deletions expected for this cluster
	deletions, ok := e.deletions[cluster]
	if !ok {
		return true, nil
	}
	for pod, uid := range deletions {
		canRemove, err := checker.CanRemoveExpectation(pod, uid)
		if err != nil {
			return false, err
		}
		if canRemove {
			delete(deletions, pod)
		} else {
			return false, nil
		}
	}
	return len(deletions) == 0, nil
}

// -- Generations expectations

func (e *Expectations) ExpectGeneration(meta metav1.ObjectMeta) {
	e.generations[meta.UID] = meta.Generation
}

func (e *Expectations) ExpectedGeneration(metaObjs ...metav1.ObjectMeta) bool {
	for _, meta := range metaObjs {
		if expectedGen, exists := e.generations[meta.UID]; exists && meta.Generation < expectedGen {
			return false
		}
	}
	return true
}

// GetGenerations returns the map of generations, for testing purpose mostly.
func (e *Expectations) GetGenerations() map[types.UID]int64 {
	return e.generations
}
