// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package expectations

import (
	"errors"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
func (e *Expectations) ExpectDeletion(pod corev1.Pod) {
	cluster, exists := getClusterFromPodLabel(pod)
	if !exists {
		return // Should not happen as all Pods should have the correct labels
	}
	var expectedDeletions map[types.NamespacedName]types.UID
	expectedDeletions, exists = e.deletions[cluster]
	if !exists {
		expectedDeletions = map[types.NamespacedName]types.UID{}
		e.deletions[cluster] = expectedDeletions
	}
	expectedDeletions[k8s.ExtractNamespacedName(&pod)] = pod.UID
}

// CancelExpectedDeletion removes an expected deletion for the given Pod.
func (e *Expectations) CancelExpectedDeletion(pod corev1.Pod) {
	cluster, exists := getClusterFromPodLabel(pod)
	if !exists {
		return // Should not happen as all Pods should have the correct labels
	}
	var expectedDeletions map[types.NamespacedName]types.UID
	expectedDeletions, exists = e.deletions[cluster]
	if !exists {
		return
	}
	delete(expectedDeletions, k8s.ExtractNamespacedName(&pod))
}

func getClusterFromPodLabel(pod corev1.Pod) (types.NamespacedName, bool) {
	cluster, exists := label.ClusterFromResourceLabels(pod.GetObjectMeta())
	if !exists {
		log.Error(errors.New("cannot find the cluster label on Pod"),
			"Failed to get cluster from Pod annotation",
			"pod_name", pod.Name,
			"pod_namespace", pod.Namespace)
	}
	return cluster, exists
}

// SatisfiedDeletions uses the provided DeletionChecker to check if the delete expectations are satisfied.
func (e *Expectations) SatisfiedDeletions(client k8s.Client, cluster types.NamespacedName) (bool, error) {
	// Get all the deletions expected for this cluster
	deletions, ok := e.deletions[cluster]
	if !ok {
		return true, nil
	}
	for pod, uid := range deletions {
		canRemove, err := canRemoveExpectation(client, pod, uid)
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
func canRemoveExpectation(client k8s.Client, podName types.NamespacedName, uid types.UID) (bool, error) {
	// Try to get the Pod
	var currentPod corev1.Pod
	err := client.Get(podName, &currentPod)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return currentPod.UID != uid, nil
}

// -- Generations expectations

func (e *Expectations) ExpectGeneration(meta metav1.ObjectMeta) {
	e.generations[meta.UID] = meta.Generation
}

func (e *Expectations) SatisfiedGenerations(metaObjs ...metav1.ObjectMeta) bool {
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
