// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/finalizer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	"k8s.io/apimachinery/pkg/types"
)

// watchName names the license watch for a specific ES cluster.
func watchName(esName types.NamespacedName) string {
	return esName.Name + "-license-watch"
}

// ensureLicenseWatch ensures a watch is registered for a given clusters license
func ensureLicenseWatch(esName types.NamespacedName, w watches.DynamicWatches) error {
	return w.ClusterLicense.AddHandler(watches.NamedWatch{
		Name:    watchName(esName),
		Watcher: esName,
		Watched: esName, // license has the same name as the cluster
	})
}

// Finalizer ensures any registered license watches are removed on cluster deletion.
func Finalizer(esName types.NamespacedName, w watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "licenses.finalizers.elasticsearch.k8s.elastic.co",
		Execute: func() error {
			w.ClusterLicense.RemoveHandlerForKey(watchName(esName))
			return nil
		},
	}
}
