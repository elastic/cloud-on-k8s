package watches

import (
	"sync"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	//SecretWatch TODO don't use a global variable for this, this is just for demonstration.
	SecretWatch = NewDynamicEnqueueRequest()
	log         = logf.Log.WithName("dynamic-enqueue-request")
)

// ToReconcileRequestTransformer is the handler/transformer registration that can be added or removed
// from DynamicEnqueueRequest.
type ToReconcileRequestTransformer interface {
	// Key identifies the transformer
	Key() string
	// ToReconcileRequest transforms the event for object to one or many reconcile.Request if relevant.
	ToReconcileRequest(object metav1.Object) []reconcile.Request
}

// NewDynamicEnqueueRequest creates a new DynamicEnqueueRequest
func NewDynamicEnqueueRequest() *DynamicEnqueueRequest {
	return &DynamicEnqueueRequest{
		transformers: make(map[string]ToReconcileRequestTransformer),
	}
}

// DynamicEnqueueRequest is an EventHandler that allows addition and removal of
// request transformers at runtime allowing dynamic reconciliation based on specific resources.
type DynamicEnqueueRequest struct {
	mutex        sync.RWMutex
	transformers map[string]ToReconcileRequestTransformer
	scheme       *runtime.Scheme
}

// AddWatch adds a new request transformer to this DynamicEnqueueRequest.
func (d *DynamicEnqueueRequest) AddWatch(xform ToReconcileRequestTransformer) error {
	if d.scheme == nil {
		return errors.New("DynamicEnqueueRequest is not initialised yet. No scheme")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	inject.SchemeInto(d.scheme, xform)
	d.transformers[xform.Key()] = xform
	log.V(4).Info("Added new transformer", "Now", d.transformers)
	return nil
}

// RemoveWatch removes the watch defined by the transformer.
func (d *DynamicEnqueueRequest) RemoveWatch(xform ToReconcileRequestTransformer) {
	d.RemoveWatchForKey(xform.Key())
}

// RemoveWatchForKey removes the watch identified by the given key.
func (d *DynamicEnqueueRequest) RemoveWatchForKey(key string) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	delete(d.transformers, key)
	log.V(4).Info("Removed transformer", "removed", key, "now", d.transformers)
}

// DynamicEnqueueRequest implements EventHandler
var _ handler.EventHandler = &DynamicEnqueueRequest{}

// Create is called in response to a create event - e.g. Pod Creation.
func (d *DynamicEnqueueRequest) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.transformers {
		for _, req := range v.ToReconcileRequest(evt.Meta) {
			log.V(4).Info("Create event transformed", "key", v.Key())
			q.Add(req)
		}
	}
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (d *DynamicEnqueueRequest) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.transformers {
		for _, req := range v.ToReconcileRequest(evt.MetaOld) {
			log.V(4).Info("Update event transformed (old)", "key", v.Key())
			q.Add(req)
		}
		for _, req := range v.ToReconcileRequest(evt.MetaNew) {
			log.V(4).Info("Update event transformed (new)", "key", v.Key())
			q.Add(req)
		}
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (d *DynamicEnqueueRequest) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.transformers {
		for _, req := range v.ToReconcileRequest(evt.Meta) {
			log.V(4).Info("Delete event transformed", "key", v.Key())
			q.Add(req)
		}
	}
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (d *DynamicEnqueueRequest) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.transformers {
		for _, req := range v.ToReconcileRequest(evt.Meta) {
			log.V(4).Info("Generic event transformed", "key", v.Key())
			q.Add(req)
		}
	}
}

// InjectScheme is used by the ControllerManager to inject Scheme into Sources, EventHandlers, Predicates, and
// Reconciles.
func (d *DynamicEnqueueRequest) InjectScheme(scheme *runtime.Scheme) error {
	d.scheme = scheme
	return nil
}

var _ inject.Scheme = &DynamicEnqueueRequest{}
