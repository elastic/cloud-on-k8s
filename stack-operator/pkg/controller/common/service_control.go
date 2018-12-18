package common

import (
	"reflect"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/reconciler"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("stack-controller")

func ReconcileService(
	c client.Client,
	scheme *runtime.Scheme,
	service *corev1.Service,
	owner v1.Object,
) (reconcile.Result, error) {
	err := reconciler.ReconcileResource(reconciler.Params{
		Client: c,
		Scheme: scheme,
		Owner:  owner,
		Object: service,
		Differ: func(expected, found *corev1.Service) bool {
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
			return !reflect.DeepEqual(expected.Spec, found.Spec)
		},
		Modifier: func(expected, found *corev1.Service) {
			found.Spec = expected.Spec // only update spec, keep the rest
		},
	})
	return reconcile.Result{}, err

}
