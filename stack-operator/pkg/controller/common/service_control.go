package common

import (
	"context"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log            = logf.Log.WithName("stack-controller")

func ReconcileService(
	c client.Client,
	scheme *runtime.Scheme,
	service *corev1.Service,
	owner v1.Object,
) (reconcile.Result, error) {
	if err := controllerutil.SetControllerReference(owner, service, scheme); err != nil {
		return reconcile.Result{}, err
	}
	// Check if already exists
	expected := service
	found := &corev1.Service{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Create if needed
		log.Info(Concat("Creating service ", expected.Namespace, "/", expected.Name))

		err = c.Create(context.TODO(), expected)
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
			Concat("Updating service ", expected.Namespace, "/", expected.Name),
		)
		found.Spec = expected.Spec // only update spec, keep the rest
		err := c.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}
