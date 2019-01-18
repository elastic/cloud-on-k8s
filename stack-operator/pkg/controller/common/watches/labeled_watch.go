package watches

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type LabeledWatch struct {
	Label string
	Watched types.NamespacedName
	Watcher types.NamespacedName
}

func (w LabeledWatch) Key() string {
	return w.Label
}

func (w LabeledWatch) ToReconcileRequest(object metav1.Object) []reconcile.Request {
	if object.GetName() == w.Watched.Name && object.GetNamespace() == w.Watched.Namespace {
		return []reconcile.Request{
			{
				NamespacedName: w.Watcher,
			},
		}
	}
	return nil
}

var _ ToReconcileRequestTransformer = &LabeledWatch{}