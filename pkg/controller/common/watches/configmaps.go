// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// WatchUserProvidedConfigMaps registers a watch for user-provided ConfigMaps.
// Only one watch per watcher is registered:
// - if it already exists with different ConfigMaps, it is replaced to watch the new ones.
// - if no ConfigMaps are provided, the watch is removed.
func WatchUserProvidedConfigMaps(
	watcher types.NamespacedName,
	watched DynamicWatches,
	watchName string,
	configMapNsns []types.NamespacedName,
) error {
	if len(configMapNsns) == 0 {
		watched.ConfigMaps.RemoveHandlerForKey(watchName)
		return nil
	}
	return watched.ConfigMaps.AddHandler(NamedWatch[*corev1.ConfigMap]{
		Name:    watchName,
		Watched: configMapNsns,
		Watcher: watcher,
	})
}
