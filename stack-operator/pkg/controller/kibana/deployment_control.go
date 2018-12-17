package kibana

import (
	"reflect"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/reconciler"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	defaultRevisionHistoryLimit int32 = 0
)

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

// ReconcileDeployment upserts the given deployment for the specified owner.
func (r *ReconcileKibana) ReconcileDeployment(deploy appsv1.Deployment, owner metav1.Object) (appsv1.Deployment, error) {
	err := reconciler.ReconcileResource(reconciler.Params{
		Client: r,
		Scheme: r.scheme,
		Owner:  owner,
		Object: &deploy,
		Differ: func(expected, found *appsv1.Deployment) bool {
			return !reflect.DeepEqual(expected.Spec.Selector, found.Spec.Selector) ||
				!reflect.DeepEqual(expected.Spec.Replicas, found.Spec.Replicas) ||
				!reflect.DeepEqual(expected.Spec.Template.ObjectMeta, found.Spec.Template.ObjectMeta) ||
				!reflect.DeepEqual(expected.Spec.Template.Spec.Containers[0].Name, found.Spec.Template.Spec.Containers[0].Name) ||
				!reflect.DeepEqual(expected.Spec.Template.Spec.Containers[0].Env, found.Spec.Template.Spec.Containers[0].Env) ||
				!reflect.DeepEqual(expected.Spec.Template.Spec.Containers[0].Image, found.Spec.Template.Spec.Containers[0].Image)
			// TODO: do something better than reflect.DeepEqual above?
			// TODO: containers[0] is a bit flaky
			// TODO: technically not only the Spec may be different, but deployment labels etc.
		},
		Modifier: func(expected, found *appsv1.Deployment) {
			// Update the found object and write the result back if there are any changes
			found.Spec = expected.Spec
		},
	})
	return deploy, err

}
