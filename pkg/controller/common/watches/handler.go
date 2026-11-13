// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/metrics"
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
	EventHandler() handler.TypedEventHandler[T, reconcile.Request]
}

// Values of the "handler" label of elastic_dynamic_watches_registered.
const (
	configMapsHandlerKind          = "configmaps"
	secretsHandlerKind             = "secrets"
	servicesHandlerKind            = "services"
	podsHandlerKind                = "pods"
	referencedResourcesHandlerKind = "referencedresources"
	unknownHandlerKind             = "unknown"
)

// handlerKindFor returns the "handler" label value for the watched type T.
func handlerKindFor[T client.Object]() string {
	var zero T
	switch any(zero).(type) {
	case *corev1.ConfigMap:
		return configMapsHandlerKind
	case *corev1.Secret:
		return secretsHandlerKind
	case *corev1.Service:
		return servicesHandlerKind
	case *corev1.Pod:
		return podsHandlerKind
	case nil:
		// client.Object is used for the generic referenced-resources watch.
		return referencedResourcesHandlerKind
	default:
		return unknownHandlerKind
	}
}

// NewDynamicEnqueueRequest creates a new DynamicEnqueueRequest
func NewDynamicEnqueueRequest[T client.Object](ctrName string) *DynamicEnqueueRequest[T] {
	de := &DynamicEnqueueRequest[T]{
		ctrName:       ctrName,
		handlerKind:   handlerKindFor[T](),
		registrations: make(map[string]HandlerRegistration[T]),
	}
	_ = de.getDynamicWatchesMetric()
	return de
}

// DynamicEnqueueRequest is an EventHandler that allows addition and removal of
// event handler registrations at runtime allowing dynamic reconciliation based on specific resources.
type DynamicEnqueueRequest[T client.Object] struct {
	mutex         sync.RWMutex
	registrations map[string]HandlerRegistration[T]
	// handlerKind is exposed as the "handler" label of the dynamic watches metric.
	handlerKind string
	// ctrName is exposed as the "controller" label of the dynamic watches metric.
	ctrName string
}

// initMetric initializes the metric with the handler label.
func (d *DynamicEnqueueRequest[T]) getDynamicWatchesMetric() prometheus.Gauge {
	return metrics.DynamicWatchesRegistered.WithLabelValues(d.ctrName, d.handlerKind)
}

func (d *DynamicEnqueueRequest[T]) setDynamicWatchesValue() {
	g := d.getDynamicWatchesMetric()
	g.Set(float64(len(d.registrations)))
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
	if exists {
		log.V(1).Info("Replacing existing handler registration", "key", handler.Key())
	} else {
		log.V(1).Info("Adding new handler registration", "key", handler.Key(), "current_registrations_keys", maps.Keys(d.registrations))
	}
	d.registrations[handler.Key()] = handler
	d.setDynamicWatchesValue()
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
	if _, exists := d.registrations[key]; exists {
		delete(d.registrations, key)
		d.setDynamicWatchesValue()
	}
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
var _ handler.TypedEventHandler[client.Object, reconcile.Request] = (*DynamicEnqueueRequest[client.Object])(nil)

// Create is called in response to a create event - e.g. Pod Creation.
func (d *DynamicEnqueueRequest[T]) Create(ctx context.Context, evt event.TypedCreateEvent[T], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Create(ctx, evt, q)
	}
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (d *DynamicEnqueueRequest[T]) Update(ctx context.Context, evt event.TypedUpdateEvent[T], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Update(ctx, evt, q)
	}
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (d *DynamicEnqueueRequest[T]) Delete(ctx context.Context, evt event.TypedDeleteEvent[T], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Delete(ctx, evt, q)
	}
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (d *DynamicEnqueueRequest[T]) Generic(ctx context.Context, evt event.TypedGenericEvent[T], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _, v := range d.registrations {
		v.EventHandler().Generic(ctx, evt, q)
	}
}
