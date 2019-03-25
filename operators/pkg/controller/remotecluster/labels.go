// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// RemoteClusterDynamicWatchesFinalizer designates a finalizer to clean up unused watches.
	RemoteClusterDynamicWatchesFinalizer = "dynamic-watches.remotecluster.k8s.elastic.co"
	// RemoteClusterNamespaceLabelName used to represent the name of a local cluster in a relationship.
	RemoteClusterNamespaceLabelName = "elasticsearch.k8s.elastic.co/remote-cluster-name"
	// RemoteClusterNameLabelName used to represent the namespace of a local cluster in a relationship.
	RemoteClusterNameLabelName = "elasticsearch.k8s.elastic.co/remote-cluster-namespace"
)

func trustRelationshipObjectMeta(
	name string,
	owner *v1alpha1.RemoteCluster,
	local v1alpha1.ObjectSelector,
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
