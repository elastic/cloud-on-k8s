// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Changes represents the changes to perform on the Elasticsearch pods
type Changes struct {
	ToCreate PodsToCreate
	ToDelete PodsToDelete
	ToKeep   pod.PodsWithConfig
}

// PodToCreate defines a pod to be created, along with
// the reasons why it doesn't match any existing pod
type PodToCreate struct {
	Pod             corev1.Pod
	PodSpecCtx      pod.PodSpecContext
	MismatchReasons map[string][]string
}

// PodsToCreate is simply a list of PodToCreate
type PodsToCreate []PodToCreate

// Pods is a helper method to retrieve pods only (no spec context or mismatch reasons)
func (p PodsToCreate) Pods() []corev1.Pod {
	pods := make([]corev1.Pod, len(p))
	for i, pod := range p {
		pods[i] = pod.Pod
	}
	return pods
}

// PodToDelete defines a pod to be deleted, and indicates
// whether its PVC should be kept around for reuse.
type PodToDelete struct {
	pod.PodWithConfig
	ReusePVC bool
}

// PodsToDelete is a list of PodToDelete.
type PodsToDelete []PodToDelete

// Pods is a helper method to retrieve pods only.
func (p PodsToDelete) Pods() []corev1.Pod {
	pods := make([]corev1.Pod, len(p))
	for i, pod := range p {
		pods[i] = pod.Pod
	}
	return pods
}

// PodsWithConfig is a helper method to retrieve PodsWithConfig only.
func (p PodsToDelete) PodsWithConfig() pod.PodsWithConfig {
	pods := make([]pod.PodWithConfig, len(p))
	for i, pod := range p {
		pods[i] = pod.PodWithConfig
	}
	return pods
}

// filterPVCReuse returns a filtered list of pods to delete according to the given bool.
func (p PodsToDelete) filterPVCReuse(withPVCReuse bool) PodsToDelete {
	filtered := make(PodsToDelete, 0, len(p))
	for _, toDelete := range p {
		if toDelete.ReusePVC == withPVCReuse {
			filtered = append(filtered, toDelete)
		}
	}
	return filtered
}

// WithPVCReuse returns a filtered list of pods to delete, marked for PVC reuse.
func (p PodsToDelete) WithPVCReuse() PodsToDelete {
	return p.filterPVCReuse(true)
}

// WithPVCReuse returns a filtered list of pods to delete, not marked for PVC reuse..
func (p PodsToDelete) WithoutPVCReuse() PodsToDelete {
	return p.filterPVCReuse(false)
}

// EmptyChanges creates an empty Changes with empty arrays (not nil)
func EmptyChanges() Changes {
	return Changes{
		ToCreate: []PodToCreate{},
		ToKeep:   pod.PodsWithConfig{},
		ToDelete: []PodToDelete{},
	}
}

// HasChanges returns true if there are no topology changes to performed
func (c Changes) HasChanges() bool {
	return len(c.ToCreate) > 0 || len(c.ToDelete) > 0
}

// HasMasterChanges returns true if some masters are involved in the topology changes.
func (c Changes) HasMasterChanges() bool {
	for _, pod := range c.ToCreate {
		if label.IsMasterNode(pod.Pod) {
			return true
		}
	}
	for _, pod := range c.ToDelete {
		if label.IsMasterNode(pod.Pod) {
			return true
		}
	}
	return false
}

// IsEmpty returns true if this set has no deletion, creation or kept pods
func (c Changes) IsEmpty() bool {
	return len(c.ToCreate) == 0 && len(c.ToDelete) == 0 && len(c.ToKeep) == 0
}

// Copy copies this Changes. It copies the underlying slices and maps, but not their contents
func (c Changes) Copy() Changes {
	res := Changes{
		ToCreate: append([]PodToCreate{}, c.ToCreate...),
		ToKeep:   append(pod.PodsWithConfig{}, c.ToKeep...),
		ToDelete: append([]PodToDelete{}, c.ToDelete...),
	}
	return res
}

// Group groups the current changes into groups based on the GroupingDefinitions
func (c Changes) Group(
	groupingDefinitions []v1alpha1.GroupingDefinition,
	remainingPodsState PodsState,
) (ChangeGroups, error) {
	remainingChanges := c.Copy()
	groups := make([]ChangeGroup, 0, len(groupingDefinitions)+1)

	for i, gd := range groupingDefinitions {
		group := ChangeGroup{
			Name: indexedGroupName(i),
		}
		selector, err := metav1.LabelSelectorAsSelector(&gd.Selector)
		if err != nil {
			return nil, err
		}

		group.Changes, remainingChanges = remainingChanges.Partition(selector)
		if group.Changes.IsEmpty() {
			// selector does not match anything
			continue
		}
		group.PodsState, remainingPodsState = remainingPodsState.Partition(group.Changes)
		groups = append(groups, group)
	}

	if !remainingChanges.IsEmpty() {
		// remaining changes do not match any group definition selector, group them together as a single group
		groups = append(groups, ChangeGroup{
			Name:      UnmatchedGroupName,
			PodsState: remainingPodsState,
			Changes:   remainingChanges,
		})
	}

	return groups, nil
}

// Partition divides changes into 2 changes based on the given selector:
// changes that match the selector, and changes that don't
func (c Changes) Partition(selector labels.Selector) (Changes, Changes) {
	matchingChanges := EmptyChanges()
	remainingChanges := EmptyChanges()

	for _, toKeep := range c.ToKeep {
		if selector.Matches(labels.Set(toKeep.Pod.Labels)) {
			matchingChanges.ToKeep = append(matchingChanges.ToKeep, toKeep)
		} else {
			remainingChanges.ToKeep = append(remainingChanges.ToKeep, toKeep)
		}
	}
	for _, toDelete := range c.ToDelete {
		if selector.Matches(labels.Set(toDelete.Pod.Labels)) {
			matchingChanges.ToDelete = append(matchingChanges.ToDelete, toDelete)
		} else {
			remainingChanges.ToDelete = append(remainingChanges.ToDelete, toDelete)
		}
	}
	for _, toCreate := range c.ToCreate {
		if selector.Matches(labels.Set(toCreate.Pod.Labels)) {
			matchingChanges.ToCreate = append(matchingChanges.ToCreate, toCreate)
		} else {
			remainingChanges.ToCreate = append(remainingChanges.ToCreate, toCreate)
		}
	}

	return matchingChanges, remainingChanges
}
