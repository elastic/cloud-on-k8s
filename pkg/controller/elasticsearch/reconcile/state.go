// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reconcile

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

var log = ulog.Log.WithName("elasticsearch-controller")

// State holds the accumulated state during the reconcile loop including the response and a copy of the
// Elasticsearch resource from the start of reconciliation, for status updates.
type State struct {
	*events.Recorder
	*StatusReporter
	cluster esv1.Elasticsearch
	status  esv1.ElasticsearchStatus
	hints   hints.OrchestrationsHints
}

// NewState creates a new reconcile state based on the given cluster
func NewState(c esv1.Elasticsearch) (*State, error) {
	hints, err := hints.NewFromAnnotations(c.Annotations)
	if err != nil {
		return nil, err
	}
	status := *c.Status.DeepCopy()
	status.ObservedGeneration = c.Generation
	// reset the health to 'unknown' so that if reconciliation fails before the observer has had a chance to get it,
	// we stop reporting a health that may be out of date
	status.Health = esv1.ElasticsearchUnknownHealth
	// reset the phase to an empty string so that we do not report an outdated phase given that certain phases are
	// stickier than others (eg. invalid)
	status.Phase = ""
	return &State{
		Recorder: events.NewRecorder(),
		StatusReporter: &StatusReporter{
			DownscaleReporter: &DownscaleReporter{},
			UpscaleReporter:   &UpscaleReporter{},
			UpgradeReporter:   &UpgradeReporter{},
		},
		cluster: c,
		status:  status,
		hints:   hints,
	}, nil
}

// MustNewState like NewState but panics on error. Use recommended only in test code.
func MustNewState(c esv1.Elasticsearch) *State {
	state, err := NewState(c)
	if err != nil {
		panic(err)
	}
	return state
}

func (s *State) fetchMinRunningVersion(resourcesState ResourcesState) (*version.Version, error) {
	minPodVersion, err := version.MinInPods(resourcesState.AllPods, label.VersionLabelName)
	if err != nil {
		log.Error(err, "failed to parse running Pods version", "namespace", s.cluster.Namespace, "es_name", s.cluster.Name)
		return nil, err
	}
	minSsetVersion, err := version.MinInStatefulSets(resourcesState.StatefulSets, label.VersionLabelName)
	if err != nil {
		log.Error(err, "failed to parse running Pods version", "namespace", s.cluster.Namespace, "es_name", s.cluster.Name)
		return nil, err
	}

	if minPodVersion == nil {
		return minSsetVersion, nil
	}
	if minSsetVersion == nil {
		return minPodVersion, nil
	}

	if minPodVersion.GT(*minSsetVersion) {
		return minSsetVersion, nil
	}

	return minPodVersion, nil
}

func (s *State) UpdateClusterHealth(clusterHealth esv1.ElasticsearchHealth) *State {
	if clusterHealth == "" {
		s.status.Health = esv1.ElasticsearchUnknownHealth
		return s
	}
	s.status.Health = clusterHealth
	return s
}

func (s *State) UpdateWithPhase(
	phase esv1.ElasticsearchOrchestrationPhase,
) *State {
	switch {
	// do not overwrite the Invalid marker
	case s.status.Phase == esv1.ElasticsearchResourceInvalid:
		return s
	// do not overwrite non-ready phases like MigratingData
	case s.status.Phase != "" && phase == esv1.ElasticsearchApplyingChangesPhase:
		return s
	}
	s.status.Phase = phase
	return s
}

func (s *State) UpdateAvailableNodes(
	resourcesState ResourcesState,
) *State {
	s.status.AvailableNodes = int32(len(AvailableElasticsearchNodes(resourcesState.CurrentPods)))
	return s
}

func (s *State) UpdateMinRunningVersion(
	resourcesState ResourcesState,
) *State {
	lowestVersion, err := s.fetchMinRunningVersion(resourcesState)
	if err != nil {
		// error already handled in fetchMinRunningVersion, move on with the status update
	} else if lowestVersion != nil {
		s.status.Version = lowestVersion.String()
	}
	// Update the related condition.
	if s.status.Version == "" {
		s.ReportCondition(esv1.RunningDesiredVersion, corev1.ConditionUnknown, "No running version reported")
		return s
	}

	desiredVersion, err := version.Parse(s.cluster.Spec.Version)
	if err != nil {
		s.ReportCondition(esv1.RunningDesiredVersion, corev1.ConditionUnknown, fmt.Sprintf("Error while parsing desired version: %s", err.Error()))
		return s
	}

	runningVersion, err := version.Parse(s.status.Version)
	if err != nil {
		s.ReportCondition(esv1.RunningDesiredVersion, corev1.ConditionUnknown, fmt.Sprintf("Error while parsing running version: %s", err.Error()))
		return s
	}

	if desiredVersion.GT(runningVersion) {
		s.ReportCondition(
			esv1.RunningDesiredVersion,
			corev1.ConditionFalse,
			fmt.Sprintf("Upgrading from %s to %s", runningVersion.String(), desiredVersion.String()),
		)
		return s
	}
	s.ReportCondition(esv1.RunningDesiredVersion, corev1.ConditionTrue, fmt.Sprintf("All nodes are running version %s", runningVersion))

	return s
}

// UpdateElasticsearchInvalidWithEvent is a convenient method to set the phase to esv1.ElasticsearchResourceInvalid
// and generate an event at the same time.
func (s *State) UpdateElasticsearchInvalidWithEvent(msg string) {
	s.status.Phase = esv1.ElasticsearchResourceInvalid
	s.AddEvent(corev1.EventTypeWarning, events.EventReasonValidation, msg)
}

// Apply takes the current Elasticsearch status, compares it to the previous status, and updates the status accordingly.
// It returns the events to emit and an updated version of the Elasticsearch cluster resource with
// the current status applied to its status sub-resource.
func (s *State) Apply() ([]events.Event, *esv1.Elasticsearch) {
	previous := s.cluster.Status
	current := s.MergeStatusReportingWith(s.status)
	if reflect.DeepEqual(previous, current) {
		return s.Events(), nil
	}
	if current.IsDegraded(previous) {
		s.AddEvent(corev1.EventTypeWarning, events.EventReasonUnhealthy, "Elasticsearch cluster health degraded")
	}
	s.cluster.Status = current
	return s.Events(), &s.cluster
}

// UpdateOrchestrationHints updates the orchestration hints collected so far with the hints in hint.
func (s *State) UpdateOrchestrationHints(hint hints.OrchestrationsHints) {
	s.hints = s.hints.Merge(hint)
}

// OrchestrationHints returns the current annotation hints as maintained in reconciliation state. Initially these will be
// populated from the Elasticsearch resource. But after calls to UpdateOrchestrationHints they can deviate from the state
// stored in the API server.
func (s *State) OrchestrationHints() hints.OrchestrationsHints {
	return s.hints
}
