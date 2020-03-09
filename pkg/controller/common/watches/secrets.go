// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package watches

import (
	"k8s.io/apimachinery/pkg/types"
)

// WatchUserProvidedAuth registers a watch for user-provided secrets.
// Only one watch per watcher is registered:
// - if it already exists with different secrets, it is replaced to watch the new secrets.
// - if there is non secrets provided by the user, remove the watch.
func WatchUserProvidedSecrets(
	watcher types.NamespacedName, // resources to which the watches are attached (eg. an Elasticsearch object)
	watched DynamicWatches, // resources already watched by watched
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
