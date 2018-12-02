package elasticsearch

import (
	"fmt"
	"reflect"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/events"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Event is a k8s event that can be recorded via an event recorder.
type Event struct {
	EventType string
	Reason    string
	Message   string
}

// ReconcileState holds the accumulated state during the reconcile loop including the response and a pointer to an
// Elasticsearch resource for status updates.
type ReconcileState struct {
	cluster  v1alpha1.ElasticsearchCluster
	status   v1alpha1.ElasticsearchStatus
	result   reconcile.Result
	events   []Event
}

// NewReconcileState creates a new reconcile state based on the given request and Elasticsearch resource with the
// resource state reset to empty.
func NewReconcileState(c v1alpha1.ElasticsearchCluster) ReconcileState {
	return ReconcileState{cluster: c, status: *c.Status.DeepCopy(),}
}

// AvailableElasticsearchNodes filters a slice of pods for the ones that are ready.
func AvailableElasticsearchNodes(pods []corev1.Pod) []corev1.Pod {
	var nodesAvailable []corev1.Pod
	for _, pod := range pods {
		conditionsTrue := 0
		for _, cond := range pod.Status.Conditions {
			if cond.Status == corev1.ConditionTrue && (cond.Type == corev1.ContainersReady || cond.Type == corev1.PodReady) {
				conditionsTrue++
			}
		}
		if conditionsTrue == 2 {
			nodesAvailable = append(nodesAvailable, pod)
		}
	}
	return nodesAvailable
}

func (s *ReconcileState) Result() reconcile.Result {
	return s.result
}

// UpdateElasticsearchState updates the Elasticsearch section of the state resource status based on the given pods.
func (s *ReconcileState) UpdateElasticsearchState(
	state support.ResourcesState,
) {
	s.status.ClusterUUID = state.ClusterState.ClusterUUID
	s.status.MasterNode = state.ClusterState.MasterNodeName()
	s.status.AvailableNodes = len(AvailableElasticsearchNodes(state.CurrentPods))
	s.status.Health = v1alpha1.ElasticsearchHealth("unknown")
	if s.status.Phase == "" {
		s.status.Phase = v1alpha1.ElasticsearchOperationalPhase
	}
	if state.ClusterHealth.Status != "" {
		s.status.Health = v1alpha1.ElasticsearchHealth(state.ClusterHealth.Status)
	}
}

// UpdateElasticsearchPending marks Elasticsearch as being the pending phase in the resource status.
func (s *ReconcileState) UpdateElasticsearchPending(result reconcile.Result, pods []corev1.Pod) {
	s.status.AvailableNodes = len(AvailableElasticsearchNodes(pods))
	s.status.Phase = v1alpha1.ElasticsearchPendingPhase
	s.status.Health = v1alpha1.ElasticsearchRedHealth
	s.result = result
}

// UpdateElasticsearchMigrating marks Elasticsearch as being in the data migration phase in the resource status.
func (s *ReconcileState) UpdateElasticsearchMigrating(
	result reconcile.Result,
	state support.ResourcesState,
) {
	s.AddEvent(
		corev1.EventTypeNormal,
		events.EventReasonDelayed,
		"Requested topology change delayed by data migration",
	)
	s.status.Phase = v1alpha1.ElasticsearchMigratingDataPhase
	s.result = result
	s.UpdateElasticsearchState(state)
}

func (s *ReconcileState) AddEvent(eventType, reason, message string) {
	s.events = append(s.events, Event{
		eventType,
		reason,
		message,
	})
}

func (s *ReconcileState) Apply() ([]Event, *v1alpha1.ElasticsearchCluster) {
	previous := s.cluster.Status
	if reflect.DeepEqual(previous, s.status) {
		return s.events, nil
	}
	if s.status.IsDegraded(previous) {
		s.AddEvent(corev1.EventTypeWarning, events.EventReasonUnhealthy, "ElasticsearchCluster health degraded")
	}
	oldUUID := previous.ClusterUUID
	newUUID := s.status.ClusterUUID
	if newUUID == "" {
		// don't record false positives when the cluster is temporarily unavailable
		s.status.ClusterUUID = oldUUID
		newUUID = oldUUID
	}
	if newUUID != oldUUID {
		s.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected,
			fmt.Sprintf("Cluster UUID changed (was: %s, is: %s)", oldUUID, newUUID),
		)
	}
	newMaster := s.status.MasterNode
	oldMaster := previous.MasterNode
	var masterChanged = newMaster != oldMaster && newMaster != ""
	if masterChanged {
		s.AddEvent(corev1.EventTypeNormal, events.EventReasonStateChange,
			fmt.Sprintf("Master node is now %s", newMaster),
		)
	}
	s.cluster.Status = s.status
	return s.events, &s.cluster
}
