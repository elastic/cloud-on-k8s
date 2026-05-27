// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	"context"
	"iter"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// WatchNamespaceFlips registers a source.Channel watch driven by notifier.
// Whenever a namespace's match-state flips, the mapper lists all objects of the
// kind produced by newList in that namespace and enqueues a reconcile request for
// each. No-ops when notifier is nil (legacy / static-namespace mode).
func WatchNamespaceFlips(
	c controller.Controller,
	cl client.Client,
	notifier NamespaceNotifier,
	newList func() client.ObjectList,
) error {
	if notifier == nil {
		return nil
	}
	return c.Watch(source.Channel(
		notifier.Subscribe(),
		handler.TypedEnqueueRequestsFromMapFunc[*corev1.Namespace, reconcile.Request](
			func(ctx context.Context, ns *corev1.Namespace) []reconcile.Request {
				list := newList()
				if err := cl.List(ctx, list, client.InNamespace(ns.Name)); err != nil {
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
		),
	))
}

func TypedWatchNamespaceFlips(
	c controller.Controller,
	cl client.Client,
	notifier NamespaceNotifier,
	listObjects func(ctx context.Context, cl client.Client, ns *corev1.Namespace) (iter.Seq[client.Object], error),
) error {
	if notifier == nil {
		return nil
	}
	return c.Watch(source.Channel(
		notifier.Subscribe(),
		handler.TypedEnqueueRequestsFromMapFunc[*corev1.Namespace, reconcile.Request](
			func(ctx context.Context, ns *corev1.Namespace) []reconcile.Request {
				itr, err := listObjects(ctx, cl, ns)
				if err != nil {
					return nil
				}
				var reqs []reconcile.Request
				for item := range itr {
					reqs = append(reqs, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: item.GetNamespace(),
							Name:      item.GetName(),
						},
					})
				}
				return reqs
			},
		),
	))
}

type NamespaceNotifier interface {
	Subscribe() <-chan event.TypedGenericEvent[*corev1.Namespace]
}
