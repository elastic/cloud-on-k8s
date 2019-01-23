package watches

import (
	"sync"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("dynamic-enqueue-request")
)

// HandlerRegistration is the event handler registration that can be added or removed
// from DynamicEnqueueRequest.
type HandlerRegistration interface {
	// Key identifies the transformer
	Key() string
	// EventHandler handles CRUD events and turns them into reconcile.Request if relevant.
	EventHandler() handler.EventHandler
}

// NewDynamicEnqueueRequest creates a new DynamicEnqueueRequest
func NewDynamicEnqueueRequest() *DynamicEnqueueRequest {
	return &DynamicEnqueueRequest{
		registrations: make(map[string]HandlerRegistration),
	}
}

// DynamicEnqueueRequest is an EventHandler that allows addition and removal of
// event handler registrations at runtime allowing dynamic reconciliation based on specific resources.
type DynamicEnqueueRequest struct {
	mutex         sync.RWMutex
	registrations map[string]HandlerRegistration
	scheme        *runtime.Scheme
}

// AddHandlers adds the new event handlers to this DynamicEnqueueRequest.
func (d *DynamicEnqueueRequest) AddHandlers(handlers ...HandlerRegistration) error {
	for _, h := range handlers {
		if err := d.AddHandler(h); err != nil {
			return err
		}
	}
	return nil
}

// AddHandler adds a new event handler to this DynamicEnqueueRequest.
func (d *DynamicEnqueueRequest) AddHandler(handler HandlerRegistration) error {
	if d.scheme == nil {
		return errors.New("DynamicEnqueueRequest is not initialised yet. No scheme")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	inject.SchemeInto(d.scheme, handler)
	d.registrations[handler.Key()] = handler
	log.V(4).Info("Added new handler registration", "Now", d.registrations)
	return nil
}

// RemoveHandler removes the handler defined by the transformer.
func (d *DynamicEnqueueRequest) RemoveHandler(handler HandlerRegistration) {
	d.RemoveHandlerForKey(handler.Key())
}

// RemoveHandlerForKey removes the handler identified by the given key.
func (d *DynamicEnqueueRequest) RemoveHandlerForKey(key string) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	delete(d.registrations, key)
	log.V(4).Info("Removed handler registration", "removed", key, "now", d.registrations)
}

// DynamicEnqueueRequest implements EventHandler
var _ handler.EventHandler = &DynamicEnqueueRequest{}

// Create is called in response to a create event - e.g. Pod Creation.
func (d *DynamicEnqueueRequest) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Create(evt, q)
	}
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (d *DynamicEnqueueRequest) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Update(evt, q)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (d *DynamicEnqueueRequest) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Delete(evt, q)
	}
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (d *DynamicEnqueueRequest) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Generic(evt, q)
	}
}

// InjectScheme is used by the ControllerManager to inject Scheme into Sources, EventHandlers, Predicates, and
// Reconciles.
func (d *DynamicEnqueueRequest) InjectScheme(scheme *runtime.Scheme) error {
	d.scheme = scheme
	return nil
}

var _ inject.Scheme = &DynamicEnqueueRequest{}
