package stack

import (
	"context"
	"reflect"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	defaultStrategy                appsv1.DeploymentStrategyType = "RollingUpdate"
	default25Percent                                             = intstr.FromString("25%")
	defaultProgressDeadlineSeconds int32                         = 600
	defaultRevisionHIstoryLimit    int32                         = 10
)

type DeploymentParams struct {
	Name      string
	Namespace string
	Selector  map[string]string
	Labels    map[string]string
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
			ProgressDeadlineSeconds: common.Int32(defaultProgressDeadlineSeconds),
			RevisionHistoryLimit:    common.Int32(defaultRevisionHIstoryLimit),
			Strategy: appsv1.DeploymentStrategy{
				Type: defaultStrategy,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &default25Percent,
					MaxSurge:       &default25Percent,
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: params.Selector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: params.Labels,
				},
				Spec: params.PodSpec,
			},
			Replicas: &params.Replicas,
		},
	}
}

// ReconcileDeployment upserts the given deployment for the specified stack.
func (r *ReconcileStack) ReconcileDeployment(deploy appsv1.Deployment, instance deploymentsv1alpha1.Stack) (appsv1.Deployment, error) {
	if err := controllerutil.SetControllerReference(&instance, &deploy, r.scheme); err != nil {
		return deploy, err
	}

	// Check if the Deployment already exists
	found := appsv1.Deployment{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, &found)
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
	} else if !reflect.DeepEqual(deploy.Spec, found.Spec) {
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
