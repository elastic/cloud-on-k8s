package elasticsearch

import (
	"fmt"
	"reflect"

	k8serrors "k8s.io/apimachinery/pkg/util/errors"

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

func (s *ReconcileState) updateWithPhase(
	phase v1alpha1.ElasticsearchOrchestrationPhase,
	resourcesState support.ResourcesState,
	observedState support.ObservedState,
) *ReconcileState {
	if observedState.ClusterState != nil {
		s.status.ClusterUUID = observedState.ClusterState.ClusterUUID
		s.status.MasterNode = observedState.ClusterState.MasterNodeName()
	}
	s.status.AvailableNodes = len(AvailableElasticsearchNodes(resourcesState.CurrentPods))
	s.status.Health = v1alpha1.ElasticsearchHealth("unknown")
	s.status.Phase = phase

	if observedState.ClusterHealth != nil && observedState.ClusterHealth.Status != "" {
		s.status.Health = v1alpha1.ElasticsearchHealth(observedState.ClusterHealth.Status)
	}
	return s
}

// UpdateElasticsearchState updates the Elasticsearch section of the state resource status based on the given pods.
func (s *ReconcileState) UpdateElasticsearchState(
	resourcesState support.ResourcesState,
	observedState support.ObservedState,
) *ReconcileState {
	return s.updateWithPhase(s.status.Phase, resourcesState, observedState)
}

// UpdateElasticsearchOperational marks Elasticsearch as being operational in the resource status.
func (s *ReconcileState) UpdateElasticsearchOperational(
	resourcesState support.ResourcesState,
	observedState support.ObservedState,

) *ReconcileState {
	return s.updateWithPhase(v1alpha1.ElasticsearchOperationalPhase, resourcesState, observedState)
}

// UpdateElasticsearchPending marks Elasticsearch as being the pending phase in the resource status.
func (s *ReconcileState) UpdateElasticsearchPending(pods []corev1.Pod) *ReconcileState {
	s.status.AvailableNodes = len(AvailableElasticsearchNodes(pods))
	s.status.Phase = v1alpha1.ElasticsearchPendingPhase
	s.status.Health = v1alpha1.ElasticsearchRedHealth
	return s
}

// UpdateElasticsearchMigrating marks Elasticsearch as being in the data migration phase in the resource status.
func (s *ReconcileState) UpdateElasticsearchMigrating(
	resourcesState support.ResourcesState,
	observedState support.ObservedState,
) *ReconcileState {
	s.AddEvent(
		corev1.EventTypeNormal,
		events.EventReasonDelayed,
		"Requested topology change delayed by data migration",
	)
	return s.updateWithPhase(v1alpha1.ElasticsearchMigratingDataPhase, resourcesState, observedState)
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

// ReconcileResults collects intermediate results of a reconciliation run and any errors that occured.
type ReconcileResults struct {
	results []reconcile.Result
	errors  []error
}

// WithError adds an error to the results.
func (r *ReconcileResults) WithError(err error) *ReconcileResults {
	if err != nil {
		r.errors = append(r.errors, err)
	}
	return r
}

// WithResult adds an result to the results.
func (r *ReconcileResults) WithResult(res reconcile.Result) *ReconcileResults {
	r.results = append(r.results, res)
	return r
}

// Apply applies the output of a reconciliation step to the results. The step outcome is implicitly considered
// recoverable as we just record the results and continue.
func (r *ReconcileResults) Apply(step string, recoverableStep func() (reconcile.Result, error)) *ReconcileResults {
	result, err := recoverableStep()
	if err != nil {
		log.Error(err, fmt.Sprintf("Error during %s, continuing", step))
	}
	return r.WithError(err).WithResult(result)
}

// Aggregate compares the collected results with each other and returns the most specific one.
// Where specific means requeue at a given time is more specific then generic requeue which is more specific
// than no requeue. It also returns any errors recorded.
func (r *ReconcileResults) Aggregate() (reconcile.Result, error) {
	var current reconcile.Result
	for _, next := range r.results {
		if nextResultTakesPrecedence(current, next) {
			current = next
		}
	}
	return current, k8serrors.NewAggregate(r.errors)
}

// nextResultTakesPrecedence compares the current reconciliation result with the proposed one,
// and returns true if the current result should be replaced by the proposed one.
func nextResultTakesPrecedence(current, next reconcile.Result) bool {
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
