package stack

import (
	"context"
	"reflect"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/action"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcileStack) reconcileService(stack *deploymentsv1alpha1.Stack, service *corev1.Service) (action.Interface, error) {
	if err := controllerutil.SetControllerReference(stack, service, r.scheme); err != nil {
		return action.NOOP, err
	}
	// Check if already exists
	expected := service
	found := &corev1.Service{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Create if needed
		return action.Create{Obj: expected}, nil
	} else if err != nil {
		return action.NOOP, err
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
		found.Spec = expected.Spec // only update spec, keep the rest
		return action.Update{Obj: found}, nil
	}
	return action.NOOP, nil
}

// IsPublicServiceReady checks if Elasticsearch public service is ready,
// so that the ES cluster can respond to HTTP requests.
// Here we just check that the service has endpoints to route requests to.
func (r *ReconcileStack) IsPublicServiceReady(s deploymentsv1alpha1.Stack) (bool, error) {
	endpoints := corev1.Endpoints{}
	publicService := elasticsearch.NewPublicService(s).ObjectMeta
	namespacedName := types.NamespacedName{Namespace: publicService.Namespace, Name: publicService.Name}
	err := r.Get(context.TODO(), namespacedName, &endpoints)
	if err != nil {
		return false, err
	}
	for _, subs := range endpoints.Subsets {
		if len(subs.Addresses) > 0 {
			return true, nil
		}
	}
	return false, nil
}
