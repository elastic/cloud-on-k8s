// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nsmatch"
)

// WatchNamespaceScopeChange registers a direct watch on Namespace objects
// (through the cache) and enqueues reconcile requests, via mapFn,
// whenever a namespace's match state against matcher's selector
// changes. A Create or Delete event is let through when the namespace
// currently matches; an Update event is let through when the match result
// differs between ObjectOld and ObjectNew.
func WatchNamespaceScopeChange(
	c controller.Controller,
	ch cache.Cache,
	matcher *nsmatch.NamespaceMatcher,
	mapFn func(context.Context, *corev1.Namespace) []reconcile.Request,
) error {
	if !matcher.SelectorEnabled() {
		return nil
	}

	return c.Watch(source.Kind( //nolint:forbidigo // Watch all namespaces to cover the off-boarding cases.
		ch,
		&corev1.Namespace{},
		handler.TypedEnqueueRequestsFromMapFunc(mapFn),
		predicate.TypedFuncs[*corev1.Namespace]{
			CreateFunc: func(e event.TypedCreateEvent[*corev1.Namespace]) bool {
				// A namespace observed for the first time only needs a reconcile
				// if it's already in scope; an out-of-scope namespace should not
				// trigger reconciliation.
				return matcher.NamespaceMatches(e.Object)
			},
			UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Namespace]) bool {
				// Label edits in general can be irrelevant; only a change
				// across the selector boundary changes which resources are
				// in scope, so that's the only case worth reconciling.
				return matcher.NamespaceMatches(e.ObjectOld) != matcher.NamespaceMatches(e.ObjectNew)
			},
			DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Namespace]) bool {
				// Mirrors CreateFunc: only an in-scope namespace going away
				// can leave behind managed resources that need handling.
				return matcher.NamespaceMatches(e.Object)
			},
			GenericFunc: func(e event.TypedGenericEvent[*corev1.Namespace]) bool {
				// Namespace watches never produce generic events;
				// Mirrors CreateFunc: only an in-scope namespace
				// [TODO: revisit].
				return matcher.NamespaceMatches(e.Object)
			},
		},
	))
}

// ReconcileObjectsInNamespace returns a mapFn, for use with
// WatchNamespaceScopeChange, that lists every
// object of the kind produced by newList in the given namespace and enqueues
// a reconcile request for each. Lists via ch (the cache) rather than the
// FilterClient so that objects in a namespace being de-scoped are still
// returned — the FilterClient would silently drop them because the namespace
// no longer matches the selector, hiding the reconcile requests needed to
// clean them up.
func ReconcileObjectsInNamespace(ch cache.Cache, newList func() client.ObjectList) func(context.Context, *corev1.Namespace) []reconcile.Request {
	return func(ctx context.Context, ns *corev1.Namespace) []reconcile.Request {
		list := newList()
		// Use the cache directly (not the FilterClient) so that resources in
		// namespaces being de-scoped are still visible here. The FilterClient
		// would silently drop them because the namespace no longer matches the
		// selector, causing us to miss the reconcile requests needed to clean up.
		if err := ch.List(ctx, list, client.InNamespace(ns.Name)); err != nil {
			return nil
		}
		items, err := apimeta.ExtractList(list)
		if err != nil {
			return nil
		}
		reqs := make([]reconcile.Request, 0, len(items))
		for _, item := range items {
			// ExtractList only guarantees runtime.Object; every generated
			// Kubernetes list type also satisfies client.Object in practice,
			// but this guards against a caller passing a list whose element
			// type doesn't, rather than panicking on GetNamespace/GetName below.
			obj, ok := item.(client.Object)
			if !ok {
				continue
			}
			reqs = append(reqs, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: obj.GetNamespace(),
					Name:      obj.GetName(),
				},
			})
		}
		return reqs
	}
}
