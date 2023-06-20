// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	"context"
	"sync"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

var (
	log = ulog.Log.WithName("dynamic-enqueue-request")
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
func (d *DynamicEnqueueRequest) RemoveHandler(handler HandlerRegistration) {
	d.RemoveHandlerForKey(handler.Key())
}

// RemoveHandlerForKey removes the handler identified by the given key.
func (d *DynamicEnqueueRequest) RemoveHandlerForKey(key string) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	delete(d.registrations, key)
}

// Registrations returns the list of registered handler names.
func (d *DynamicEnqueueRequest) Registrations() []string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	keys := make([]string, 0, len(d.registrations))
	for k := range d.registrations {
		keys = append(keys, k)
	}
	return keys
}

// DynamicEnqueueRequest implements EventHandler
var _ handler.EventHandler = &DynamicEnqueueRequest{}

// Create is called in response to a create event - e.g. Pod Creation.
func (d *DynamicEnqueueRequest) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Create(ctx, evt, q)
	}
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (d *DynamicEnqueueRequest) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Update(ctx, evt, q)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (d *DynamicEnqueueRequest) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Delete(ctx, evt, q)
	}
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (d *DynamicEnqueueRequest) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Generic(ctx, evt, q)
	}
}
