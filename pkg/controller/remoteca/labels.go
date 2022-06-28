// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package remoteca

import (
	"fmt"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/certificates/remoteca"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// RemoteClusterNamespaceLabelName used to represent the namespace of the RemoteCluster in a TrustRelationship.
	RemoteClusterNamespaceLabelName = "elasticsearch.k8s.elastic.co/remote-cluster-namespace"
	// RemoteClusterNameLabelName used to represent the name of the RemoteCluster in a TrustRelationship.
	RemoteClusterNameLabelName = "elasticsearch.k8s.elastic.co/remote-cluster-name"
	// remoteCASecretSuffix is the suffix added to the aforementioned Secret.
	remoteCASecretSuffix = "remote-ca"
)

func remoteCAObjectMeta(
	name string,
	owner *esv1.Elasticsearch,
	remote types.NamespacedName,
) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: owner.Namespace,
		Labels: maps.Merge(
			map[string]string{
				RemoteClusterNamespaceLabelName: remote.Namespace,
				RemoteClusterNameLabelName:      remote.Name,
			},
			remoteca.Labels(owner.Name),
		),
	}
}

// RemoteCASecretName returns the name of the Secret that contains the transport CA of a remote cluster
func remoteCASecretName(
	localClusterName string,
	remoteCluster types.NamespacedName,
) string {
	return esv1.ESNamer.Suffix(
		fmt.Sprintf("%s-%s-%s", localClusterName, remoteCluster.Namespace, remoteCluster.Name),
		remoteCASecretSuffix,
	)
}
