// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NamedWatch is an event handler that allows watching a specific resource identified by
// Watched. Events will be handled by Watcher.
type NamedWatch[T client.Object] struct {
	// Name identifies this watch for easier removal and deduplication.
	Name string
	// Watched are the resources being watched.
	Watched []types.NamespacedName
	// Watcher is the receiver of the reconcile.Request
	Watcher types.NamespacedName
}

var _ handler.EventHandler = &NamedWatch[client.Object]{}

func (w NamedWatch[T]) Create(_ context.Context, evt event.TypedCreateEvent[T], q workqueue.RateLimitingInterface) {
	for _, req := range w.toReconcileRequest(evt.Object) {
		q.Add(req)
	}
}

func (w NamedWatch[T]) Update(_ context.Context, evt event.TypedUpdateEvent[T], q workqueue.RateLimitingInterface) {
	for _, req := range w.toReconcileRequest(evt.ObjectOld) {
		q.Add(req)
	}
	for _, req := range w.toReconcileRequest(evt.ObjectNew) {
		q.Add(req)
	}
}

func (w NamedWatch[T]) Delete(_ context.Context, evt event.TypedDeleteEvent[T], q workqueue.RateLimitingInterface) {
	for _, req := range w.toReconcileRequest(evt.Object) {
		q.Add(req)
	}
}

func (w NamedWatch[T]) Generic(_ context.Context, evt event.TypedGenericEvent[T], q workqueue.RateLimitingInterface) {
	for _, req := range w.toReconcileRequest(evt.Object) {
		q.Add(req)
	}
}

func (w NamedWatch[T]) EventHandler() handler.TypedEventHandler[T] {
	return w
}

// Key identifies this transformer.
func (w NamedWatch[T]) Key() string {
	return w.Name
}

// EventHandler transforms the event for object to one or many reconcile.Request if relevant.
func (w NamedWatch[T]) toReconcileRequest(object metav1.Object) []reconcile.Request {
	for _, watched := range w.Watched {
		if object.GetName() == watched.Name && object.GetNamespace() == watched.Namespace {
			return []reconcile.Request{
				{
					NamespacedName: w.Watcher,
				},
			}
		}
	}
	return nil
}

var _ HandlerRegistration[client.Object] = &NamedWatch[client.Object]{}
