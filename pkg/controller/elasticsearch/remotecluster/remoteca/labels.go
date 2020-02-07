// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remoteca

import (
	"fmt"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// RemoteClusterNamespaceLabelName used to represent the namespace of the RemoteCluster in a TrustRelationship.
	RemoteClusterNamespaceLabelName = "remotecluster.k8s.elastic.co/namespace"
	// RemoteClusterNameLabelName used to represent the name of the RemoteCluster in a TrustRelationship.
	RemoteClusterNameLabelName = "remotecluster.k8s.elastic.co/name"
	// TypeLabelValue is a type used to .
	TypeLabelValue = "remote-ca"

	certsInternalSecretName = "remote-ca"
)

func remoteCAObjectMeta(
	name string,
	owner *esv1.Elasticsearch,
	remote types.NamespacedName,
) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: owner.Namespace,
		Labels: map[string]string{
			RemoteClusterNamespaceLabelName: remote.Namespace,
			RemoteClusterNameLabelName:      remote.Name,
			label.ClusterNameLabelName:      owner.Name,
			common.TypeLabelName:            TypeLabelValue,
		},
	}
}

// RemoteCASecretName returns the name of the Secret that contains the transport CA of a remote cluster
func remoteCASecretName(
	localClusterName string,
	remoteCluster types.NamespacedName,
) string {
	return esv1.ESNamer.Suffix(
		fmt.Sprintf("%s-%s-%s", localClusterName, remoteCluster.Namespace, remoteCluster.Name),
		certsInternalSecretName,
	)
}

func GetRemoteCAMatchingLabel(esName string) client.MatchingLabels {
	return map[string]string{
		label.ClusterNameLabelName: esName,
		common.TypeLabelName:       TypeLabelValue,
	}
}
