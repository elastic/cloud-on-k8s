package watches

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// LabeledWatch is a request transformer that allows watching a specific resource identified by
// Watched. Events will be handled by Watcher.
type LabeledWatch struct {
	// Label identifies this watch for easier removal and deduplication.
	Label string
	// Watched is the resource being watched.
	Watched types.NamespacedName
	// Watcher is the receiver of the reconcile.Request
	Watcher types.NamespacedName
}

// Key identifies this transformer.
func (w LabeledWatch) Key() string {
	return w.Label
}

// ToReconcileRequest transforms the event for object to one or many reconcile.Request if relevant.
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
