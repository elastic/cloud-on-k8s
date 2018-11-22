package stack

import (
	"context"
	"reflect"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/action"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/kibana"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	defaultRevisionHistoryLimit int32 = 0
)

// DeploymentParams describe the attributes of a deployment relevant for reconciliation.
type DeploymentParams struct {
	Name      string
	Namespace string
	Selector  map[string]string
	Labels    map[string]string
	PodLabels map[string]string
	Replicas  int32
	PodSpec   corev1.PodSpec
}

// NewDeployment creates a Deployment API struct with the given PodSpec.
func NewDeployment(params DeploymentParams) appsv1.Deployment {
	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: params.Namespace,
			Labels:    params.Labels,
		},
		Spec: appsv1.DeploymentSpec{
			RevisionHistoryLimit: common.Int32(defaultRevisionHistoryLimit),
			Selector: &metav1.LabelSelector{
				MatchLabels: params.Selector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: params.PodLabels,
				},
				Spec: params.PodSpec,
			},
			Replicas: &params.Replicas,
		},
	}
}

// ReconcileDeployment upserts the given deployment for the specified stack.
func (r *ReconcileStack) ReconcileDeployment(deploy appsv1.Deployment, instance deploymentsv1alpha1.Stack) ([]action.Interface, error) {
	var actions []action.Interface
	if err := controllerutil.SetControllerReference(&instance, &deploy, r.scheme); err != nil {
		return actions, err
	}

	// Check if the Deployment already exists
	found := appsv1.Deployment{}
	actions = append(actions, kibana.UpdateKibanaStatus{Deployment: &found})
	err := r.Get(context.TODO(), types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, &found)
	if err != nil && errors.IsNotFound(err) {
		actions = append(actions, action.Create{Obj: &deploy})
	} else if err != nil {
		return actions, err
	} else if !reflect.DeepEqual(deploy.Spec, found.Spec) {
		// Update the found object and write the result back if there are any changes
		found.Spec = deploy.Spec
		actions = append(actions, action.Update{Obj: &found})
	}
	return actions, nil
}
