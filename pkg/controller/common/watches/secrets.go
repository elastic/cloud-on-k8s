// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
)

// WatchUserProvidedSecrets registers a watch for user-provided secrets that are in the same namespace as the watcher.
// Only one watch per watcher is registered:
// - if it already exists with different secrets, it is replaced to watch the new secrets.
// - if there is no secret provided by the user, remove the watch.
func WatchUserProvidedSecrets(
	watcher types.NamespacedName, // resource to which the watches are attached (e.g. an Elasticsearch object)
	watched DynamicWatches, // existing dynamic watches
	watchName string, // dynamic watch to register
	secrets []string, // user-provided secrets to watch
) error {
	secretSources := make([]commonv1.NamespacedSecretSource, 0, len(secrets))
	for _, secretName := range secrets {
		secretSources = append(secretSources, commonv1.NamespacedSecretSource{Namespace: watcher.Namespace, SecretName: secretName})
	}
	return WatchUserProvidedNamespacedSecrets(watcher, watched, watchName, secretSources)
}

// WatchUserProvidedNamespacedSecrets registers a watch for user-provided secrets that are in any namespace.
// Only one watch per watcher is registered:
// - if it already exists with different secrets, it is replaced to watch the new secrets.
// - if there is no secret provided by the user, remove the watch.
func WatchUserProvidedNamespacedSecrets(
	watcher types.NamespacedName, // resource to which the watches are attached (e.g. an Elasticsearch object)
	watched DynamicWatches, // existing dynamic watches
	watchName string, // dynamic watch to register
	secrets []commonv1.NamespacedSecretSource, // secrets to watch
) error {
	if len(secrets) == 0 {
		watched.Secrets.RemoveHandlerForKey(watchName)
		return nil
	}
	userSecretNsns := make([]types.NamespacedName, 0, len(secrets))
	for _, s := range secrets {
		userSecretNsns = append(userSecretNsns, types.NamespacedName{
			Namespace: s.Namespace,
			Name:      s.SecretName,
		})
	}
	return watched.Secrets.AddHandler(NamedWatch[*corev1.Secret]{
		Name:    watchName,
		Watched: userSecretNsns,
		Watcher: watcher,
	})
}

// WatchSoftOwnedSecrets triggers reconciliations on secrets referencing a soft owner.
func WatchSoftOwnedSecrets(mgr manager.Manager, c controller.Controller, ownerKind string) error {
	return c.Watch(
		source.Kind(mgr.GetCache(), &corev1.Secret{}, handler.TypedEnqueueRequestsFromMapFunc[*corev1.Secret](reconcileReqForSoftOwner(ownerKind))),
	)
}

func reconcileReqForSoftOwner(kind string) handler.TypedMapFunc[*corev1.Secret, reconcile.Request] {
	return handler.TypedMapFunc[*corev1.Secret, reconcile.Request](func(ctx context.Context, object *corev1.Secret) []reconcile.Request {
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
	})
}
