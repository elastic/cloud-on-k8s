package elasticsearch

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ReconcileState holds the accumulated state during the reconcile loop including the response and a pointer to an
// Elasticsearch resource for status updates.
type ReconcileState struct {
	Elasticsearch *v1alpha1.ElasticsearchCluster
	Result        reconcile.Result
	Request       reconcile.Request
}

// NewReconcileState creates a new reconcile state based on the given request and Elasticsearch resource with the
// resource state reset to empty.
func NewReconcileState(request reconcile.Request, es *v1alpha1.ElasticsearchCluster) ReconcileState {
	// reset status to reconstruct it during the reconcile cycle
	es.Status = v1alpha1.ElasticsearchStatus{}
	return ReconcileState{Request: request, Elasticsearch: es}
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

// UpdateElasticsearchState updates the Elasticsearch section of the state resource status based on the given pods.
func (s ReconcileState) UpdateElasticsearchState(
	state support.ResourcesState,
) {
	s.Elasticsearch.Status.ClusterUUID = state.ClusterState.ClusterUUID
	s.Elasticsearch.Status.MasterNode = state.ClusterState.MasterNodeName()
	s.Elasticsearch.Status.AvailableNodes = len(AvailableElasticsearchNodes(state.CurrentPods))
	s.Elasticsearch.Status.Health = v1alpha1.ElasticsearchHealth("unknown")
	if s.Elasticsearch.Status.Phase == "" {
		s.Elasticsearch.Status.Phase = v1alpha1.ElasticsearchOperationalPhase
	}
	if state.ClusterHealth.Status != "" {
		s.Elasticsearch.Status.Health = v1alpha1.ElasticsearchHealth(state.ClusterHealth.Status)
	}
}

// UpdateElasticsearchPending marks Elasticsearch as being the pending phase in the resource status.
func (s ReconcileState) UpdateElasticsearchPending(result reconcile.Result, pods []corev1.Pod) {
	s.Elasticsearch.Status.AvailableNodes = len(AvailableElasticsearchNodes(pods))
	s.Elasticsearch.Status.Phase = v1alpha1.ElasticsearchPendingPhase
	s.Elasticsearch.Status.Health = v1alpha1.ElasticsearchRedHealth
	s.Result = result
}

// UpdateElasticsearchMigrating marks Elasticsearch as being in the data migration phase in the resource status.
func (s ReconcileState) UpdateElasticsearchMigrating(
	result reconcile.Result,
	state support.ResourcesState,
) {
	s.Elasticsearch.Status.Phase = v1alpha1.ElasticsearchMigratingDataPhase
	s.Result = result
	s.UpdateElasticsearchState(state)
}
