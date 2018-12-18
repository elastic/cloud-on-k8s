package configmaps

import (
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/reconciler"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ReconcileConfigMap checks for an existing config map and updates it or creates one if it does not exist.
func ReconcileConfigMap(
	c client.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
	expected corev1.ConfigMap,
) error {
	return reconciler.ReconcileResource(
		reconciler.Params{
			Client: c,
			Scheme: scheme,
			Owner:  &es,
			Object: &expected,
			Differ: func(expected, found *corev1.Secret) bool {
				return !reflect.DeepEqual(expected.Data, found.Data)
			},
			Modifier: func(expected, found *corev1.ConfigMap) {
				found.Data = expected.Data
			},
		},
	)
}
