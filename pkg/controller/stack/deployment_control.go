package stack

import (
	"context"
	"fmt"
	"reflect"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewDeployment creates a Deployment API struct with the given PodSpec.
func NewDeployment(name string, namespace string, spec corev1.PodSpec) appsv1.Deployment {
	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-deployment",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"deployment": name + "-deployment"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"deployment": name + "-deployment"}},
				Spec:       spec,
			},
		},
	}
}

// ReconcileDeployment upserts the given deployment for the specified stack.
func (r *ReconcileStack) ReconcileDeployment(deploy appsv1.Deployment, instance deploymentsv1alpha1.Stack) (reconcile.Result, error) {
	if err := controllerutil.SetControllerReference(&instance, &deploy, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if the Deployment already exists
	found := &appsv1.Deployment{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info(
			fmt.Sprintf("Creating Deployment %s/%s", deploy.Namespace, deploy.Name),
			"iteration", r.iteration,
		)
		err = r.Create(context.TODO(), &deploy)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else if err != nil {
		log.Info(fmt.Sprintf("searched deployment with %s found %s", deploy.Name, found))
		return reconcile.Result{}, err
	} else if !reflect.DeepEqual(deploy.Spec, found.Spec) {
		// Update the found object and write the result back if there are any changes
		found.Spec = deploy.Spec
		log.Info(
			fmt.Sprintf("Updating Deployment %s/%s", deploy.Namespace, deploy.Name),
			"iteration", r.iteration,
		)
		err = r.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil

}
