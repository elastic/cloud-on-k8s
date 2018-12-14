package kibana

import (
	"context"
	"reflect"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	if err := controllerutil.SetControllerReference(owner, &deploy, r.scheme); err != nil {
		return deploy, err
	}

	// Check if the Deployment already exists
	found := appsv1.Deployment{}
	err := r.Get(context.TODO(), k8s.ToNamespacedName(deploy.ObjectMeta), &found)
	if err != nil && errors.IsNotFound(err) {
		log.Info(
			common.Concat("Creating Deployment ", deploy.Namespace, "/", deploy.Name),
			"iteration", r.iteration,
		)
		err = r.Create(context.TODO(), &deploy)
		if err != nil {
			return deploy, err
		}
	} else if err != nil {
		log.Info(common.Concat("searched deployment ", deploy.Name, " found ", found.Name))
		return found, err
	} else if !reflect.DeepEqual(deploy.Spec.Selector, found.Spec.Selector) ||
		!reflect.DeepEqual(deploy.Spec.Replicas, found.Spec.Replicas) ||
		!reflect.DeepEqual(deploy.Spec.Template.ObjectMeta, found.Spec.Template.ObjectMeta) ||
		!reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Name, found.Spec.Template.Spec.Containers[0].Name) ||
		!reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Env, found.Spec.Template.Spec.Containers[0].Env) ||
		!reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Image, found.Spec.Template.Spec.Containers[0].Image) {
		// TODO: do something better than reflect.DeepEqual above?
		// TODO: containers[0] is a bit flaky
		// TODO: technically not only the Spec may be different, but deployment labels etc.
		// Update the found object and write the result back if there are any changes
		found.Spec = deploy.Spec
		log.Info(
			common.Concat("Updating Deployment ", deploy.Namespace, "/", deploy.Name),
			"iteration", r.iteration,
		)
		err = r.Update(context.TODO(), &found)
		if err != nil {
			return found, err
		}
	}
	return found, nil

}
