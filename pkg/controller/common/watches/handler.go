// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	"context"
	"sync"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

var (
	log = ulog.Log.WithName("dynamic-enqueue-request")
)

// HandlerRegistration is the event handler registration that can be added or removed
// from DynamicEnqueueRequest.
type HandlerRegistration[T client.Object] interface {
	// Key identifies the transformer
	Key() string
	// EventHandler handles CRUD events and turns them into reconcile.Request if relevant.
	EventHandler() handler.TypedEventHandler[T]
}

// NewDynamicEnqueueRequest creates a new DynamicEnqueueRequest
func NewDynamicEnqueueRequest[T client.Object]() *DynamicEnqueueRequest[T] {
	return &DynamicEnqueueRequest[T]{
		registrations: make(map[string]HandlerRegistration[T]),
	}
}

// DynamicEnqueueRequest is an EventHandler that allows addition and removal of
// event handler registrations at runtime allowing dynamic reconciliation based on specific resources.
type DynamicEnqueueRequest[T client.Object] struct {
	mutex         sync.RWMutex
	registrations map[string]HandlerRegistration[T]
}

// AddHandlers adds the new event handlers to this DynamicEnqueueRequest.
func (d *DynamicEnqueueRequest[T]) AddHandlers(handlers ...HandlerRegistration[T]) error {
	for _, h := range handlers {
		if err := d.AddHandler(h); err != nil {
			return err
		}
	}
	return nil
}

// AddHandler adds a new event handler to this DynamicEnqueueRequest.
func (d *DynamicEnqueueRequest[T]) AddHandler(handler HandlerRegistration[T]) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	_, exists := d.registrations[handler.Key()]
	if !exists {
		log.V(1).Info("Adding new handler registration", "key", handler.Key(), "current_registrations", d.registrations)
	}
	d.registrations[handler.Key()] = handler
	return nil
}

// RemoveHandler removes the handler defined by the transformer.
func (d *DynamicEnqueueRequest[T]) RemoveHandler(handler HandlerRegistration[T]) {
	d.RemoveHandlerForKey(handler.Key())
}

// RemoveHandlerForKey removes the handler identified by the given key.
func (d *DynamicEnqueueRequest[T]) RemoveHandlerForKey(key string) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	delete(d.registrations, key)
}

// Registrations returns the list of registered handler names.
func (d *DynamicEnqueueRequest[T]) Registrations() []string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	keys := make([]string, 0, len(d.registrations))
	for k := range d.registrations {
		keys = append(keys, k)
	}
	return keys
}

// DynamicEnqueueRequest implements TypedEventHandler
var _ handler.TypedEventHandler[client.Object] = &DynamicEnqueueRequest[client.Object]{}

// Create is called in response to a create event - e.g. Pod Creation.
func (d *DynamicEnqueueRequest[T]) Create(ctx context.Context, evt event.TypedCreateEvent[T], q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Create(ctx, evt, q)
	}
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (d *DynamicEnqueueRequest[T]) Update(ctx context.Context, evt event.TypedUpdateEvent[T], q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Update(ctx, evt, q)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (d *DynamicEnqueueRequest[T]) Delete(ctx context.Context, evt event.TypedDeleteEvent[T], q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Delete(ctx, evt, q)
	}
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (d *DynamicEnqueueRequest[T]) Generic(ctx context.Context, evt event.TypedGenericEvent[T], q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Generic(ctx, evt, q)
	}
}
