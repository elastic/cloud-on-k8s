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
	cluster v1alpha1.ElasticsearchCluster
	status  v1alpha1.ElasticsearchStatus
	result  reconcile.Result
	events  []Event
}

// NewReconcileState creates a new reconcile state based on the given request and Elasticsearch resource with the
// resource state reset to empty.
func NewReconcileState(c v1alpha1.ElasticsearchCluster) ReconcileState {
	return ReconcileState{cluster: c, status: *c.Status.DeepCopy()}
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

// Result returns the current reconcile result.
func (s *ReconcileState) Result() reconcile.Result {
	return s.result
}

func (s *ReconcileState) updateWithPhase(phase v1alpha1.ElasticsearchOrchestrationPhase, state support.ResourcesState) *ReconcileState {
	s.status.ClusterUUID = state.ClusterState.ClusterUUID
	s.status.MasterNode = state.ClusterState.MasterNodeName()
	s.status.AvailableNodes = len(AvailableElasticsearchNodes(state.CurrentPods))
	s.status.Health = v1alpha1.ElasticsearchHealth("unknown")
	s.status.Phase = phase

	if state.ClusterHealth.Status != "" {
		s.status.Health = v1alpha1.ElasticsearchHealth(state.ClusterHealth.Status)
	}
	return s
}

// UpdateElasticsearchState updates the Elasticsearch section of the state resource status based on the given pods.
func (s *ReconcileState) UpdateElasticsearchState(
	state support.ResourcesState,
) *ReconcileState {
	return s.updateWithPhase(v1alpha1.ElasticsearchOperationalPhase, state)
}

func (s *ReconcileState) UpdateWithResult(result reconcile.Result) *ReconcileState {
	if s.nextResultTakesPrecedence(result) {
		s.result = result
	}
	return s
}

// UpdateElasticsearchPending marks Elasticsearch as being the pending phase in the resource status.
func (s *ReconcileState) UpdateElasticsearchPending(result reconcile.Result, pods []corev1.Pod) *ReconcileState {
	s.status.AvailableNodes = len(AvailableElasticsearchNodes(pods))
	s.status.Phase = v1alpha1.ElasticsearchPendingPhase
	s.status.Health = v1alpha1.ElasticsearchRedHealth
	return s.UpdateWithResult(result)
}

// UpdateElasticsearchMigrating marks Elasticsearch as being in the data migration phase in the resource status.
func (s *ReconcileState) UpdateElasticsearchMigrating(
	result reconcile.Result,
	state support.ResourcesState,
) *ReconcileState {
	s.AddEvent(
		corev1.EventTypeNormal,
		events.EventReasonDelayed,
		"Requested topology change delayed by data migration",
	)
	s.status.Phase = v1alpha1.ElasticsearchMigratingDataPhase
	s.UpdateWithResult(result)
	return s.updateWithPhase(v1alpha1.ElasticsearchMigratingDataPhase, state)
}

// AddEvent records the intent to emit a k8s event with the given attributes.
func (s *ReconcileState) AddEvent(eventType, reason, message string) *ReconcileState {
	s.events = append(s.events, Event{
		eventType,
		reason,
		message,
	})
	return s
}

// Apply takes the current Elasticsearch status, compares it to the previous status, and updates the status accordingly.
// It returns the events to emit and an updated version of the Elasticsearch cluster resource with
// the current status applied to its status sub-resource.
func (s *ReconcileState) Apply() ([]Event, *v1alpha1.ElasticsearchCluster) {
	previous := s.cluster.Status
	current := s.status
	if reflect.DeepEqual(previous, current) {
		return s.events, nil
	}
	if current.IsDegraded(previous) {
		s.AddEvent(corev1.EventTypeWarning, events.EventReasonUnhealthy, "Elasticsearch cluster health degraded")
	}
	oldUUID := previous.ClusterUUID
	newUUID := current.ClusterUUID
	if newUUID == "" {
		// don't record false positives when the cluster is temporarily unavailable
		current.ClusterUUID = oldUUID
		newUUID = oldUUID
	}
	if newUUID != oldUUID {
		s.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected,
			fmt.Sprintf("Cluster UUID changed (was: %s, is: %s)", oldUUID, newUUID),
		)
	}
	newMaster := current.MasterNode
	oldMaster := previous.MasterNode
	// empty master means loss of master node or no valid cluster data
	// we record it in status but don't emit an event. This might be transient but is a valid state
	// as opposed to the same thing for the cluster UUID where we are interested in the eventual loss of state
	// and want to ignore intermediate 'empty' states
	var masterChanged = newMaster != oldMaster && newMaster != ""
	if masterChanged {
		s.AddEvent(corev1.EventTypeNormal, events.EventReasonStateChange,
			fmt.Sprintf("Master node is now %s", newMaster),
		)
	}
	s.cluster.Status = current
	return s.events, &s.cluster
}

// nextResultTakesPrecedence compares the current reconciliation result with the proposed one,
// and returns true if the current result should be replaced by the proposed one.
func (s *ReconcileState) nextResultTakesPrecedence(next reconcile.Result) bool {
	current := s.result
	if current == next {
		return false // no need to replace the result
	}
	if next.Requeue && !current.Requeue && current.RequeueAfter <= 0 {
		return true // next requests requeue current does not, next takes precendence
	}
	if next.RequeueAfter > 0 && (current.RequeueAfter == 0 || next.RequeueAfter < current.RequeueAfter) {
		return true // next requests a requeue and current does not or wants it only later
	}
	return false //default case
}
