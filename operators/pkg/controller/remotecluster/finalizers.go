// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	assoctype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/associations/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
)

// watchFinalizer ensure that we remove watches for Secrets  we are no longer interested in
// because the RemoteCluster has been deleted.
func watchFinalizer(
	clusterAssociation v1alpha1.RemoteCluster,
	local, remote assoctype.ObjectSelector,
	w watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: RemoteClusterDynamicWatchesFinalizer,
		Execute: func() error {
			w.Secrets.RemoveHandlerForKey(watchName(clusterAssociation, local))
			w.Secrets.RemoveHandlerForKey(watchName(clusterAssociation, remote))
			return nil
		},
	}
}
