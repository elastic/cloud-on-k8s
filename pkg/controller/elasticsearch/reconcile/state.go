// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconcile

import (
	"reflect"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/validation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
)

// State holds the accumulated state during the reconcile loop including the response and a pointer to an
// Elasticsearch resource for status updates.
type State struct {
	*events.Recorder
	cluster v1beta1.Elasticsearch
	status  v1beta1.ElasticsearchStatus
}

// NewState creates a new reconcile state based on the given cluster
func NewState(c v1beta1.Elasticsearch) *State {
	return &State{Recorder: events.NewRecorder(), cluster: c, status: *c.Status.DeepCopy()}
}

// AvailableElasticsearchNodes filters a slice of pods for the ones that are ready.
func AvailableElasticsearchNodes(pods []corev1.Pod) []corev1.Pod {
	var nodesAvailable []corev1.Pod
	for _, pod := range pods {
		if k8s.IsPodReady(pod) {
			nodesAvailable = append(nodesAvailable, pod)
		}
	}
	return nodesAvailable
}

func (s *State) updateWithPhase(
	phase v1beta1.ElasticsearchOrchestrationPhase,
	resourcesState ResourcesState,
	observedState observer.State,
) *State {
	s.status.AvailableNodes = len(AvailableElasticsearchNodes(resourcesState.CurrentPods))
	s.status.Phase = phase

	s.status.Health = v1beta1.ElasticsearchUnknownHealth
	if observedState.ClusterHealth != nil && observedState.ClusterHealth.Status != "" {
		s.status.Health = v1beta1.ElasticsearchHealth(observedState.ClusterHealth.Status)
	}
	return s
}

// UpdateElasticsearchState updates the Elasticsearch section of the state resource status based on the given pods.
func (s *State) UpdateElasticsearchState(
	resourcesState ResourcesState,
	observedState observer.State,
) *State {
	return s.updateWithPhase(s.status.Phase, resourcesState, observedState)
}

// UpdateElasticsearchReady marks Elasticsearch as being ready in the resource status.
func (s *State) UpdateElasticsearchReady(
	resourcesState ResourcesState,
	observedState observer.State,
) *State {
	return s.updateWithPhase(v1beta1.ElasticsearchReadyPhase, resourcesState, observedState)
}

// IsElasticsearchReady reports if Elasticsearch is ready.
func (s *State) IsElasticsearchReady(observedState observer.State) bool {
	return s.status.Phase == v1beta1.ElasticsearchReadyPhase
}

// UpdateElasticsearchApplyingChanges marks Elasticsearch as being the applying changes phase in the resource status.
func (s *State) UpdateElasticsearchApplyingChanges(pods []corev1.Pod) *State {
	s.status.AvailableNodes = len(AvailableElasticsearchNodes(pods))
	s.status.Phase = v1beta1.ElasticsearchApplyingChangesPhase
	s.status.Health = v1beta1.ElasticsearchRedHealth
	return s
}

// UpdateElasticsearchMigrating marks Elasticsearch as being in the data migration phase in the resource status.
func (s *State) UpdateElasticsearchMigrating(
	resourcesState ResourcesState,
	observedState observer.State,
) *State {
	s.AddEvent(
		corev1.EventTypeNormal,
		events.EventReasonDelayed,
		"Requested topology change delayed by data migration",
	)
	return s.updateWithPhase(v1beta1.ElasticsearchMigratingDataPhase, resourcesState, observedState)
}

// Apply takes the current Elasticsearch status, compares it to the previous status, and updates the status accordingly.
// It returns the events to emit and an updated version of the Elasticsearch cluster resource with
// the current status applied to its status sub-resource.
func (s *State) Apply() ([]events.Event, *v1beta1.Elasticsearch) {
	previous := s.cluster.Status
	current := s.status
	if reflect.DeepEqual(previous, current) {
		return s.Events(), nil
	}
	if current.IsDegraded(previous) {
		s.AddEvent(corev1.EventTypeWarning, events.EventReasonUnhealthy, "Elasticsearch cluster health degraded")
	}
	s.cluster.Status = current
	return s.Events(), &s.cluster
}

func (s *State) UpdateElasticsearchInvalid(results []validation.Result) {
	s.status.Phase = v1beta1.ElasticsearchResourceInvalid
	for _, r := range results {
		s.AddEvent(corev1.EventTypeWarning, events.EventReasonValidation, r.Reason)
	}
}
