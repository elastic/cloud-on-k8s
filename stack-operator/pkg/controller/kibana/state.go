package kibana

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/kibana/v1alpha1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ReconcileState holds the accumulated state during the reconcile loop including the response and a pointer to a Kibana
// resource for status updates.
type ReconcileState struct {
	Kibana  *v1alpha1.Kibana
	Result  reconcile.Result
	Request reconcile.Request

	originalKibana *v1alpha1.Kibana
}

// NewReconcileState creates a new reconcile state based on the given request and Kibana resource with the resource
// state reset to empty.
func NewReconcileState(request reconcile.Request, kb *v1alpha1.Kibana) ReconcileState {
	return ReconcileState{Request: request, Kibana: kb, originalKibana: kb.DeepCopy()}
}

// UpdateKibanaState updates the Kibana status based on the given deployment.
func (s ReconcileState) UpdateKibanaState(deployment v1.Deployment) {
	s.Kibana.Status.AvailableNodes = int(deployment.Status.AvailableReplicas) // TODO lossy type conversion
	s.Kibana.Status.Health = v1alpha1.KibanaRed
	for _, c := range deployment.Status.Conditions {
		if c.Type == v1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
			s.Kibana.Status.Health = v1alpha1.KibanaGreen
		}
	}
}
