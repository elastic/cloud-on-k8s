// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package expectations

import (
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// ExpectedGenerations stores UID of Pods that we did delete, but whose deletion may not be
// done yet, or not visible yet in the cache.
// It allows making sure we're not working with an out-of-date list of Pods that includes
// Pods which do not exist anymore.
type ExpectedDeletions struct {
	client       k8s.Client
	podDeletions map[types.NamespacedName]types.UID
}

// NewExpectedDeletions returns an initialized ExpectedDeletions.
func NewExpectedDeletions(client k8s.Client) *ExpectedDeletions {
	return &ExpectedDeletions{
		client:       client,
		podDeletions: make(map[types.NamespacedName]types.UID),
	}
}

// ExpectDeletion registers an expected deletion for the given Pod.
func (e *ExpectedDeletions) ExpectDeletion(pod corev1.Pod) {
	e.podDeletions[k8s.ExtractNamespacedName(&pod)] = pod.UID
}

// CancelExpectedDeletion removes an expected deletion for the given Pod.
// It's mostly used if a deletion call failed, but the expectations was already registered.
func (e *ExpectedDeletions) CancelExpectedDeletion(pod corev1.Pod) {
	delete(e.podDeletions, k8s.ExtractNamespacedName(&pod))
}

// DeletionsSatisfied ensures all registered Pods deletions are satisfied: meaning
// the corresponding Pods do not exist in the cache anymore.
// Expectations are cleared once fulfilled.
func (e *ExpectedDeletions) DeletionsSatisfied() (bool, error) {
	for pod, uid := range e.podDeletions {
		isDeleted, err := podDeleted(e.client, pod, uid)
		if err != nil {
			return false, err
		}
		if isDeleted {
			// expectation fulfilled
			delete(e.podDeletions, pod)
		} else {
			return false, nil
		}
	}
	return len(e.podDeletions) == 0, nil
}

// podDeleted returns true if the pod can be considered as deleted.
func podDeleted(client k8s.Client, pod types.NamespacedName, uid types.UID) (bool, error) {
	var podInCache corev1.Pod
	err := client.Get(pod, &podInCache)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// pod is removed
			return true, nil
		}
		return false, err
	}
	// pod may have been recreated with a different UID, which accounts for a deletion
	return podInCache.UID != uid, nil
}
