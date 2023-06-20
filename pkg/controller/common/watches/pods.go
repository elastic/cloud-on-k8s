// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func Add(mgr manager.Manager, c controller.Controller, owner client.Object, objects ...client.Object) error {
	if err := c.Watch(
		source.Kind(mgr.GetCache(), owner), &handler.EnqueueRequestForObject{},
	); err != nil {
		return err
	}
	for _, object := range objects {
		if err := c.Watch(
			source.Kind(mgr.GetCache(), object),
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), owner, handler.OnlyControllerOwner()),
		); err != nil {
			return err
		}
	}
	return nil
}

// WatchPods updates the given controller to enqueue reconciliation requests triggered by changes on Pods.
// The resource to reconcile is identified by a label on the Pods.
func WatchPods(mgr manager.Manager, c controller.Controller, objNameLabel string) error {
	return c.Watch(
		source.Kind(mgr.GetCache(), &corev1.Pod{}),
		handler.EnqueueRequestsFromMapFunc(objToReconcileRequest(objNameLabel)),
	)
}

// objToReconcileRequest returns a function to enqueue reconcile requests for the resource name set at objNameLabel.
func objToReconcileRequest(objNameLabel string) handler.MapFunc {
	return func(ctx context.Context, object client.Object) []reconcile.Request {
		labels := object.GetLabels()
		objectName, isSet := labels[objNameLabel]
		if !isSet {
			return nil
		}
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: object.GetNamespace(),
					Name:      objectName,
				},
			},
		}
	}
}
