// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	commonv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// RemoteClusterDynamicWatchesFinalizer designates a finalizer to clean up unused watches.
	RemoteClusterDynamicWatchesFinalizer = "dynamic-watches.remotecluster.k8s.elastic.co"
	// RemoteClusterNamespaceLabelName used to represent the namespace of the RemoteCluster in a TrustRelationship.
	RemoteClusterNamespaceLabelName = "remotecluster.k8s.elastic.co/namespace"
	// RemoteClusterNameLabelName used to represent the name of the RemoteCluster in a TrustRelationship.
	RemoteClusterNameLabelName = "remotecluster.k8s.elastic.co/name"
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
