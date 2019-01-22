package watches

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NamedWatch is a request transformer that allows watching a specific resource identified by
// Watched. Events will be handled by Watcher.
type NamedWatch struct {
	// Name identifies this watch for easier removal and deduplication.
	Name string
	// Watched is the resource being watched.
	Watched types.NamespacedName
	// Watcher is the receiver of the reconcile.Request
	Watcher types.NamespacedName
}

// Key identifies this transformer.
func (w NamedWatch) Key() string {
	return w.Name
}

// ToReconcileRequest transforms the event for object to one or many reconcile.Request if relevant.
func (w NamedWatch) ToReconcileRequest(object metav1.Object) []reconcile.Request {
	if object.GetName() == w.Watched.Name && object.GetNamespace() == w.Watched.Namespace {
		return []reconcile.Request{
			{
				NamespacedName: w.Watcher,
			},
		}
	}
	return nil
}

var _ ToReconcileRequestTransformer = &NamedWatch{}
