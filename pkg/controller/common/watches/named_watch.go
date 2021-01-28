// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

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
	// Watched are the resources being watched.
	Watched []types.NamespacedName
	// Watcher is the receiver of the reconcile.Request
	Watcher types.NamespacedName
}

var _ handler.EventHandler = &NamedWatch{}

func (w NamedWatch) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	for _, req := range w.toReconcileRequest(evt.Object) {
		q.Add(req)
	}
}

func (w NamedWatch) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	for _, req := range w.toReconcileRequest(evt.ObjectOld) {
		q.Add(req)
	}
	for _, req := range w.toReconcileRequest(evt.ObjectNew) {
		q.Add(req)
	}
}

func (w NamedWatch) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	for _, req := range w.toReconcileRequest(evt.Object) {
		q.Add(req)
	}
}

func (w NamedWatch) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	for _, req := range w.toReconcileRequest(evt.Object) {
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

var _ HandlerRegistration = &NamedWatch{}
