package elasticsearch

import (
	"reflect"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/reconciler"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// ReconcileConfigMap checks for an existing config map and updates it or creates one if it does not exist.
func (r *ReconcileElasticsearch) ReconcileConfigMap(es v1alpha1.ElasticsearchCluster, expected corev1.ConfigMap) error {
	return reconciler.ReconcileResource(
		reconciler.Params{
			Client: r,
			Scheme: r.scheme,
			Owner:  &es,
			Object: &expected,
			Differ: func(expected, found *corev1.ConfigMap) bool {
				return !reflect.DeepEqual(expected.Data, found.Data)
			},
			Modifier: reconciler.IdentityModifier,
		},
	)
}
