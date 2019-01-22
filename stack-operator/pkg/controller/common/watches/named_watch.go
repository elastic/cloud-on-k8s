package watches

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NamedWatch is an event handler that allows watching a specific resource identified by
// Watched. Events will be handled by Watcher.
type NamedWatch struct {
	// Name identifies this watch for easier removal and deduplication.
	Name string
	// Watched is the resource being watched.
	Watched types.NamespacedName
	// Watcher is the receiver of the reconcile.Request
	Watcher types.NamespacedName
}

var _ handler.EventHandler = &NamedWatch{}

func (w NamedWatch) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	for _, req := range w.toReconcileRequest(evt.Meta) {
		log.V(4).Info("Create event transformed", "key", w.Key())
		q.Add(req)
	}
}

func (w NamedWatch) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	for _, req := range w.toReconcileRequest(evt.MetaOld) {
		log.V(4).Info("Update event transformed (old)", "key", w.Key())
		q.Add(req)
	}
	for _, req := range w.toReconcileRequest(evt.MetaNew) {
		log.V(4).Info("Update event transformed (new)", "key", w.Key())
		q.Add(req)
	}
}

func (w NamedWatch) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	for _, req := range w.toReconcileRequest(evt.Meta) {
		log.V(4).Info("Delete event transformed", "key", w.Key())
		q.Add(req)
	}
}

func (w NamedWatch) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	for _, req := range w.toReconcileRequest(evt.Meta) {
		log.V(4).Info("Generic event transformed", "key", w.Key())
		q.Add(req)
	}
}

func (w NamedWatch) EventHandler() handler.EventHandler {
	return w
}

// Key identifies this transformer.
func (w NamedWatch) Key() string {
	return w.Name
}

// EventHandler transforms the event for object to one or many reconcile.Request if relevant.
func (w NamedWatch) toReconcileRequest(object metav1.Object) []reconcile.Request {
	if object.GetName() == w.Watched.Name && object.GetNamespace() == w.Watched.Namespace {
		return []reconcile.Request{
			{
				NamespacedName: w.Watcher,
			},
		}
	}
	return nil
}

var _ HandlerRegistration = &NamedWatch{}
