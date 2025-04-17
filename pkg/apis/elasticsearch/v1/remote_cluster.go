// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/optional"
)

var (
	RemoteClusterAPIKeysMinVersion = version.MinFor(8, 10, 0)
)

// SupportsRemoteClusterAPIKeys returns true if this cluster supports connecting to a remote cluster using API keys.
func (es *Elasticsearch) SupportsRemoteClusterAPIKeys() (*optional.Bool, error) {
	if es == nil {
		return nil, nil
	}
	if es.Status.Version == "" {
		// This cluster is not reconciled yet.
		return nil, nil
	}
	esVersion, err := version.Parse(es.Status.Version)
	if err != nil {
		return nil, err
	}
	return optional.NewBool(esVersion.GTE(RemoteClusterAPIKeysMinVersion)), nil
}

// HasRemoteClusterAPIKey returns true if this cluster is connecting to a remote cluster using API keys.
func (es *Elasticsearch) HasRemoteClusterAPIKey() bool {
	if es == nil {
		return false
	}
	for _, remoteCluster := range es.Spec.RemoteClusters {
		if remoteCluster.APIKey != nil {
			return true
		}
	}
	return false
}

// RemoteClustersCount returns the number of remote clusters using only certificates and API keys.
func (es *Elasticsearch) RemoteClustersCount() (int32, int32) {
	if es == nil {
		return 0, 0
	}
	var withoutAPIKeys, withAPIKeys int32
	for _, remoteCLuster := range es.Spec.RemoteClusters {
		if remoteCLuster.APIKey == nil {
			withoutAPIKeys++
			continue
		}
		withAPIKeys++
	}
	return withoutAPIKeys, withAPIKeys
}

// RemoteClusterAPIKey defines a remote cluster API Key.
type RemoteClusterAPIKey struct {
	// Access is the name of the API Key. It is automatically generated if not set or empty.
	// +kubebuilder:validation:Required
	Access RemoteClusterAccess `json:"access,omitempty"`
}

// RemoteClusterAccess models the API key specification as documented in https://www.elastic.co/guide/en/elasticsearch/reference/current/security-api-create-cross-cluster-api-key.html
type RemoteClusterAccess struct {
	// +kubebuilder:validation:Optional
	Search *Search `json:"search,omitempty"`
	// +kubebuilder:validation:Optional
	Replication *Replication `json:"replication,omitempty"`
}

type Search struct {
	// +kubebuilder:validation:Required
	Names []string `json:"names,omitempty"`

	// +kubebuilder:validation:Optional
	FieldSecurity *FieldSecurity `json:"field_security,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Query *commonv1.Config `json:"query,omitempty"`

	// +kubebuilder:validation:Optional
	AllowRestrictedIndices bool `json:"allow_restricted_indices,omitempty"`
}

type FieldSecurity struct {
	Grant  []string `json:"grant"`
	Except []string `json:"except"`
}

type Replication struct {
	// +kubebuilder:validation:Required
	Names []string `json:"names,omitempty"`
}
