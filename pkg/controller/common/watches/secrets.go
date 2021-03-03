// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package watches

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// WatchUserProvidedSecrets registers a watch for user-provided secrets.
// Only one watch per watcher is registered:
// - if it already exists with different secrets, it is replaced to watch the new secrets.
// - if there is no secret provided by the user, remove the watch.
func WatchUserProvidedSecrets(
	watcher types.NamespacedName, // resource to which the watches are attached (eg. an Elasticsearch object)
	watched DynamicWatches, // existing dynamic watches
	watchName string, // dynamic watch to register
	secrets []string, // user-provided secrets to watch
) error {
	if len(secrets) == 0 {
		watched.Secrets.RemoveHandlerForKey(watchName)
		return nil
	}
	userSecretNsns := make([]types.NamespacedName, 0, len(secrets))
	for _, secretName := range secrets {
		userSecretNsns = append(userSecretNsns, types.NamespacedName{
			Namespace: watcher.Namespace,
			Name:      secretName,
		})
	}
	return watched.Secrets.AddHandler(NamedWatch{
		Name:    watchName,
		Watched: userSecretNsns,
		Watcher: watcher,
	})
}

// WatchSoftOwnedSecrets triggers reconciliations on secrets referencing a soft owner.
func WatchSoftOwnedSecrets(c controller.Controller, ownerKind string) error {
	return c.Watch(
		&source.Kind{Type: &corev1.Secret{}},
		handler.EnqueueRequestsFromMapFunc(reconcileReqForSoftOwner(ownerKind)),
	)
}

func reconcileReqForSoftOwner(kind string) handler.MapFunc {
	return func(object client.Object) []reconcile.Request {
		softOwner, referenced := reconciler.SoftOwnerRefFromLabels(object.GetLabels())
		if !referenced {
			return nil
		}
		if softOwner.Kind != kind {
			return nil
		}
		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{Namespace: softOwner.Namespace, Name: softOwner.Name}},
		}
	}
}
