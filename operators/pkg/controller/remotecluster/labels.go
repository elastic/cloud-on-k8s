// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// RemoteClusterDynamicWatchesFinalizer designates a finalizer to clean up unused watches.
	RemoteClusterDynamicWatchesFinalizer = "dynamic-watches.remotecluster.k8s.elastic.co"
	// RemoteClusterSeedServiceFinalizer designates a finalizer to clean up a seed Service.
	RemoteClusterSeedServiceFinalizer = "seed-service.remotecluster.k8s.elastic.co"
	// RemoteClusterNamespaceLabelName used to represent the namespace of the RemoteCluster in a TrustRelationship.
	RemoteClusterNamespaceLabelName = "remotecluster.k8s.elastic.co/namespace"
	// RemoteClusterNameLabelName used to represent the name of the RemoteCluster in a TrustRelationship.
	RemoteClusterNameLabelName = "remotecluster.k8s.elastic.co/name"
	// RemoteClusterSeedServiceForLabelName is used to mark a service as used as a seed service for remote clusters.
	RemoteClusterSeedServiceForLabelName = "remotecluster.k8s.elastic.co/seed-service-for"
)

func trustRelationshipObjectMeta(
	name string,
	owner v1alpha1.RemoteCluster,
	local commonv1alpha1.ObjectSelector,
) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: local.Namespace,
		Labels: map[string]string{
			RemoteClusterNamespaceLabelName: owner.Namespace,
			RemoteClusterNameLabelName:      owner.Name,
			label.ClusterNameLabelName:      local.Name,
		},
	}
}
