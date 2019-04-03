// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/mutation/comparison"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/reconcile"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
)

// PodToReuse defines an existing pod that we'll
// reuse for a different spec, by restarting the process
// running in the pod without restarting the pod itself
type PodToReuse struct {
	// Initial (current) pod with its config
	Initial pod.PodWithConfig
	// Target pod after the pod reuse process
	Target PodToCreate
}

// withReusablePods checks if some pods to delete can be reused for pods to create.
// Matching pods can keep running with the same spec, but we'll restart
// the inner ES process with a different configuration.
func withReusablePods(changes Changes) Changes {
	// The given changes are kept unmodified, a new object is returned.
	changesCopy := changes.Copy()
	result := Changes{
		ToKeep:                    changesCopy.ToKeep,
		ToCreate:                  []PodToCreate{},
		ToReuse:                   []PodToReuse{},
		ToDelete:                  changes.ToDelete,
		RequireFullClusterRestart: changes.RequireFullClusterRestart,
	}

	for _, toCreate := range changes.ToCreate {
		canReuse, matchingPod, remainingToDelete := findReusablePod(toCreate, result.ToDelete)
		if canReuse {
			result.ToReuse = append(result.ToReuse, PodToReuse{
				Initial: matchingPod,
				Target:  toCreate,
			})
			// update list of pods to delete to remove the matching one
			result.ToDelete = remainingToDelete
		} else {
			// cannot reuse, should be created
			result.ToCreate = append(result.ToCreate, toCreate)
		}
	}
	return result
}

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
