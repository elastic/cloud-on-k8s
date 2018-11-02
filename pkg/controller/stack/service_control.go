package stack

import (
	"context"
	"fmt"
	"reflect"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileStack) reconcileService(stack *deploymentsv1alpha1.Stack, service *corev1.Service) (reconcile.Result, error) {
	if err := controllerutil.SetControllerReference(stack, service, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	// Check if already exists
	expected := service
	found := &corev1.Service{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Create if needed
		log.Info(fmt.Sprintf("Creating service %s/%s\n", expected.Namespace, expected.Name),
			"iteration", r.iteration,
		)

		err = r.Create(context.TODO(), expected)
		if err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// ClusterIP might not exist in the expected service,
	// but might have been set after creation by k8s on the actual resource.
	// In such case, we want to use these values for comparison.
	if expected.Spec.ClusterIP == "" {
		expected.Spec.ClusterIP = found.Spec.ClusterIP
	}
	// same for the target port and node port
	if len(expected.Spec.Ports) == len(found.Spec.Ports) {
		for i := range expected.Spec.Ports {
			if expected.Spec.Ports[i].TargetPort.IntValue() == 0 {
				expected.Spec.Ports[i].TargetPort = found.Spec.Ports[i].TargetPort
			}
			if expected.Spec.Ports[i].NodePort == 0 {
				expected.Spec.Ports[i].NodePort = found.Spec.Ports[i].NodePort
			}
		}
	}

	// Update if needed
	if !reflect.DeepEqual(expected.Spec, found.Spec) {
		log.Info(
			fmt.Sprintf("Updating service %s/%s\n", expected.Namespace, expected.Name),
			"iteration", r.iteration,
		)
		found.Spec = expected.Spec // only update spec, keep the rest
		err := r.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}
