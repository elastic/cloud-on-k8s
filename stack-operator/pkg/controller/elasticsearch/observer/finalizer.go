package observer

import (
	elasticsearchv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/finalizer"
)

const (
	// FinalizerName registered for each elasticsearch resource
	FinalizerName = "observer.finalizers.elasticsearch.stack.k8s.elastic.co"
)

// Finalizer returns a finalizer to be executed upon deletion of the given cluster,
// that makes sure the cluster is not observed anymore
func (m *Manager) Finalizer(es elasticsearchv1alpha1.ElasticsearchCluster) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: FinalizerName,
		Execute: func() error {
			m.StopObserving(es)
			return nil
		},
	}
}
