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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// WatchNamespaceFlips registers a source.Channel watch driven by notifier.
// Whenever a namespace's match state changes, the mapper lists all objects of the
// kind produced by newList in that namespace and enqueues a reconcile request for
// each. No-ops when notifier is nil (legacy / static-namespace mode).
func WatchNamespaceFlips(
	c controller.Controller,
	ch cache.Cache,
	notifier NamespaceNotifier,
	newList func() client.ObjectList,
) error {
	return WatchNamespaceFlipsMapped(c, notifier,
		func(ctx context.Context, ns *corev1.Namespace) []reconcile.Request {
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
		},
	)
}

// WatchNamespaceFlipsMapped registers a source.Channel watch driven by notifier with a
// caller-provided mapper deciding which reconcile requests a namespace match-state change
// translates into. Use this instead of WatchNamespaceFlips when the objects to re-enqueue
// are not simply the ones living in the flipped namespace (e.g. associations referencing
// resources in that namespace). No-ops when notifier is nil (legacy / static-namespace mode).
func WatchNamespaceFlipsMapped(
	c controller.Controller,
	notifier NamespaceNotifier,
	mapFn func(context.Context, *corev1.Namespace) []reconcile.Request,
) error {
	if notifier == nil {
		return nil
	}
	return c.Watch(source.Channel(
		notifier.Subscribe(),
		handler.TypedEnqueueRequestsFromMapFunc(mapFn),
	))
}

type NamespaceNotifier interface {
	Subscribe() <-chan event.TypedGenericEvent[*corev1.Namespace]
	SelectorEnabled() bool
}
