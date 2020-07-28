// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package watches

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// WatchPods updates the given controller to enqueue reconciliation requests triggered by changes on Pods.
// The resource to reconcile is identified by a label on the Pods.
func WatchPods(c controller.Controller, objNameLabel string) error {
	return c.Watch(
		&source.Kind{Type: &corev1.Pod{}},
		&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(objToReconcileRequest(objNameLabel))},
	)
}

// objToReconcileRequest returns a function to enqueue reconcile requests for the resource name set at objNameLabel.
func objToReconcileRequest(objNameLabel string) func(object handler.MapObject) []reconcile.Request {
	return func(object handler.MapObject) []reconcile.Request {
		labels := object.Meta.GetLabels()
		objectName, isSet := labels[objNameLabel]
		if !isSet {
			return nil
		}
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: object.Meta.GetNamespace(),
					Name:      objectName,
				},
			},
		}
	}
}
