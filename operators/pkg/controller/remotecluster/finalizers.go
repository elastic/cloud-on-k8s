// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// watchFinalizer ensure that we remove watches for Secrets  we are no longer interested in
// because the RemoteCluster has been deleted.
func watchFinalizer(
	clusterAssociation v1alpha1.RemoteCluster,
	local, remote commonv1alpha1.ObjectSelector,
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

// seedServiceFinalizer ensures that we remove the seed service if it's no longer required
func seedServiceFinalizer(c k8s.Client, remoteCluster v1alpha1.RemoteCluster) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: RemoteClusterSeedServiceFinalizer,
		Execute: func() error {
			svc := v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: remoteCluster.Spec.Remote.K8sLocalRef.Namespace,
					Name:      remoteClusterSeedServiceName(remoteCluster.Spec.Remote.K8sLocalRef.Name),
				},
			}
			if svc.Namespace == "" {
				svc.Namespace = remoteCluster.Namespace
			}

			if err := c.Delete(&svc); err != nil && errors.IsNotFound(err) {
				return err
			}
			return nil
		},
	}
}
