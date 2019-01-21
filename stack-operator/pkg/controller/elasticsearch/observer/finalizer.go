package observer

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/finalizer"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// FinalizerName registered for each elasticsearch resource
	FinalizerName = "observer.finalizers.elasticsearch.stack.k8s.elastic.co"
)

// Finalizer returns a finalizer to be executed upon deletion of the given cluster,
// that makes sure the cluster is not observed anymore
func (m *Manager) Finalizer(cluster types.NamespacedName) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: FinalizerName,
		Execute: func() error {
			m.StopObserving(cluster)
			return nil
		},
	}
}
