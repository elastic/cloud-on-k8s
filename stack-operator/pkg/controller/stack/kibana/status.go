package kibana

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/action"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// UpdateKibanaStatus updates the stack status with Kibana status information.
type UpdateKibanaStatus struct {
	Deployment *appsv1.Deployment
}

// Name from action.Interface
func (u UpdateKibanaStatus) Name() string {
	return common.Concat("Status update for ", u.Deployment.Kind, u.Deployment.Namespace, "/", u.Deployment.Name)
}

// Execute from action.Interface
func (u UpdateKibanaStatus) Execute(ctx action.Context) (*reconcile.Result, error) {
	ctx.State.UpdateKibanaState(*u.Deployment)
	return nil, nil
}

var _ action.Interface = UpdateKibanaStatus{}
