// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package expectations

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// ExpectedPodDeletions stores UID of Pods that we did delete, but whose deletion may not be
// done yet, or not visible yet in the cache.
// It allows making sure we're not working with an out-of-date list of Pods that includes
// Pods which do not exist anymore.
type ExpectedPodDeletions struct {
	client       k8s.Client
	podDeletions map[types.NamespacedName]types.UID
}

// NewExpectedPodDeletions returns an initialized ExpectedPodDeletions.
func NewExpectedPodDeletions(client k8s.Client) *ExpectedPodDeletions {
	return &ExpectedPodDeletions{
		client:       client,
		podDeletions: make(map[types.NamespacedName]types.UID),
	}
}

// ExpectDeletion registers an expected deletion for the given Pod.
func (e *ExpectedPodDeletions) ExpectDeletion(pod corev1.Pod) {
	e.podDeletions[k8s.ExtractNamespacedName(&pod)] = pod.UID
}

// PendingPodDeletions returns a list of Pods for which deletions are not satisfied: meaning
// the corresponding Pods still exist in the cache while they should not.
// Expectations are cleared once fulfilled.
func (e *ExpectedPodDeletions) PendingPodDeletions() ([]string, error) {
	var pendingPodDeletions []string
	for pod, uid := range e.podDeletions {
		isDeleted, err := podDeleted(e.client, pod, uid)
		if err != nil {
			return nil, err
		}
		if isDeleted {
			// cache is up-to-date: expectation is fulfilled, remove it
			delete(e.podDeletions, pod)
		} else {
			pendingPodDeletions = append(pendingPodDeletions, pod.Name)
		}
	}
	return pendingPodDeletions, nil
}

// podDeleted returns true if the pod with the given UID does not exist anymore.
func podDeleted(client k8s.Client, pod types.NamespacedName, uid types.UID) (bool, error) {
	var podInCache corev1.Pod
	err := client.Get(context.Background(), pod, &podInCache)
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
