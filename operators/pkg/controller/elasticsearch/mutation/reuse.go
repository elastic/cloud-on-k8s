// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/mutation/comparison"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/reconcile"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
)

// optimizeForPodReuse checks if some pods to delete can be reused for pods to create.
// Matching pods can keep running with the same spec, but we'll restart
// the inner ES process with a different configuration.
func optimizeForPodReuse(changes Changes) Changes {
	// The given changes are kept unmodified, a new object is returned.
	result := Changes{
		ToKeep:                    changes.ToKeep,       // keep the same pods
		ToCreate:                  []PodToCreate{},      // will be filled by new pods that cannot leverage reuse
		ToReuse:                   pod.PodsWithConfig{}, // will be filled with pods to delete than we can reuse (maybe with a different config)
		ToDelete:                  changes.ToDelete,     // will be shrunk to pods to delete that cannot be reused
		RequireFullClusterRestart: changes.RequireFullClusterRestart,
	}

	for _, toCreate := range changes.ToCreate {
		canReuse, matchingPod, remainingToDelete := findReusablePod(toCreate, result.ToDelete)
		if canReuse {
			result.ToReuse = append(result.ToReuse, pod.PodWithConfig{
				// use the pod that would have been deleted
				Pod: matchingPod.Pod,
				// with the config of the pod that would have been created
				Config: toCreate.PodSpecCtx.Config,
			})
			// one more pod to delete
			result.ToDelete = remainingToDelete
		} else {
			// cannot reuse, should be created
			result.ToCreate = append(result.ToCreate, toCreate)
		}
	}

	return result
}

// findReusablePod looks for a matching pod spec in the pods to delete for the pod to create.
// It returns the matching pod along with the remaining pods to delete, without the matching one.
func findReusablePod(
	toCreate PodToCreate,
	toDelete pod.PodsWithConfig,
) (
	isMatch bool,
	matchingPod pod.PodWithConfig,
	remainingToDelete pod.PodsWithConfig,
) {

	for i, candidate := range toDelete {
		// the pod spec must be a match, but it's ok for the config to be different
		match, _ := comparison.PodMatchesSpec(candidate, toCreate.PodSpecCtx, reconcile.ResourcesState{})
		if match {
			// we found a pod we can reuse, no need to delete it anymore
			remainingToDelete = append(toDelete[:i], toDelete[i+1:]...)
			return true, candidate, remainingToDelete
		}
	}
	// cannot find a pod to reuse
	return false, pod.PodWithConfig{}, toDelete
}
