// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

import (
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// GenericEventHandler returns an EventHandler that enqueues a reconciliation request
// from the generic event NamespacedName.
func GenericEventHandler() handler.EventHandler {
	return handler.Funcs{
		GenericFunc: func(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: evt.Meta.GetNamespace(),
					Name:      evt.Meta.GetName(),
				},
			})
		},
	}
}
