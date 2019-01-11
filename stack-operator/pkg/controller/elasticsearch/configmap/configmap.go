package configmap

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewConfigMapWithData constructs a new config map with the given data
func NewConfigMapWithData(es v1alpha1.ElasticsearchCluster, data map[string]string) corev1.ConfigMap {
	return corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      es.Name,
			Namespace: es.Namespace,
			Labels:    support.NewLabels(es),
		},
		Data: data,
	}
}
