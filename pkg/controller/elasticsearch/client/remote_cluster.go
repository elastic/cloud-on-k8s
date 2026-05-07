// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
)

type RemoteClusterClient interface {
	// UpdateRemoteClusterSettings updates the remote clusters of a cluster.
	UpdateRemoteClusterSettings(context.Context, RemoteClustersSettings) error
	// GetRemoteClusterSettings retrieves the remote clusters of a cluster.
	GetRemoteClusterSettings(context.Context) (RemoteClustersSettings, error)
	// CreateCrossClusterAPIKey creates a new cross cluster API Key using the provided cross cluster API key request.
	CreateCrossClusterAPIKey(context.Context, CrossClusterAPIKeyCreateRequest) (CrossClusterAPIKeyCreateResponse, error)
	// UpdateCrossClusterAPIKey updates the cluster API Key which matches the provided ID using the provided update request.
	UpdateCrossClusterAPIKey(context.Context, string, CrossClusterAPIKeyUpdateRequest) (CrossClusterAPIKeyUpdateResponse, error)
	// InvalidateCrossClusterAPIKey invalidates a cluster API Key by its name.
	InvalidateCrossClusterAPIKey(context.Context, string) error
	// GetCrossClusterAPIKeys attempts to retrieve active Cross Cluster API Keys.
	// The provided string is used as the "name" parameter in the HTTP query.
	// Relies on the active_only parameter to only include active API Keys in the response.
	GetCrossClusterAPIKeys(context.Context, string) (CrossClusterAPIKeyList, error)
}

type CrossClusterAPIKeyInvalidateRequest struct {
	Name string `json:"name,omitempty"`
}

type CrossClusterAPIKeyCreateRequest struct {
	Name string `json:"name,omitempty"`
	CrossClusterAPIKeyUpdateRequest
}

type CrossClusterAPIKeyUpdateRequest struct {
	esv1.RemoteClusterAPIKey
	Metadata map[string]any `json:"metadata,omitempty"`
}

type CrossClusterAPIKeyCreateResponse struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
	Encoded string `json:"encoded,omitempty"`
}

type CrossClusterAPIKeyUpdateResponse struct {
	Update string `json:"string,omitempty"`
}

type CrossClusterAPIKeyList struct {
	APIKeys []CrossClusterAPIKey `json:"api_keys,omitempty"`
}

func (cl *CrossClusterAPIKeyList) Len() int {
	if cl == nil {
		return 0
	}
	return len(cl.APIKeys)
}

// GetActiveKeyWithName returns the first active key that matches the provided name or pattern.
func (cl *CrossClusterAPIKeyList) GetActiveKeyWithName(name string) *CrossClusterAPIKey {
	if cl == nil || cl.Len() == 0 {
		return nil
	}
	for _, key := range cl.APIKeys {
		if key.Name == name {
			return &key
		}
	}
	return nil
}

// KeyNames extracts the key names from a list of keys.
func (cl *CrossClusterAPIKeyList) KeyNames() sets.Set[string] {
	if cl == nil || cl.Len() == 0 {
		return nil
	}
	result := sets.New[string]()
	for _, key := range cl.APIKeys {
		result.Insert(key.Name)
	}
	return result
}

// ForCluster returns all the API keys related to a specific client cluster.
func (cl *CrossClusterAPIKeyList) ForCluster(namespace string, name string) (*CrossClusterAPIKeyList, error) {
	if cl == nil || cl.APIKeys == nil {
		return nil, nil
	}
	crossClusterAPIKeyList := &CrossClusterAPIKeyList{
		APIKeys: make([]CrossClusterAPIKey, 0, len(cl.APIKeys)),
	}
	for _, apiKey := range cl.APIKeys {
		elasticsearchName, err := apiKey.GetElasticsearchName()
		if err != nil {
			return nil, err
		}
		if elasticsearchName.Namespace == namespace && elasticsearchName.Name == name {
			crossClusterAPIKeyList.APIKeys = append(crossClusterAPIKeyList.APIKeys, apiKey)
		}
	}
	return crossClusterAPIKeyList, nil
}

type CrossClusterAPIKey struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// GetElasticsearchName returns the name of the client cluster for which this key has been created.
func (c *CrossClusterAPIKey) GetElasticsearchName() (types.NamespacedName, error) {
	if c == nil {
		return types.NamespacedName{}, nil
	}
	esNameInMetadata, ok := c.Metadata["elasticsearch.k8s.elastic.co/name"]
	if !ok {
		return types.NamespacedName{}, fmt.Errorf("missing metadata in cross cluster API key: elasticsearch.k8s.elastic.co/name")
	}
	esNamespaceInMetadata, ok := c.Metadata["elasticsearch.k8s.elastic.co/namespace"]
	if !ok {
		return types.NamespacedName{}, fmt.Errorf("missing metadata in cross cluster API key: elasticsearch.k8s.elastic.co/namespace")
	}

	namespacedName := types.NamespacedName{}
	if esName, ok := esNameInMetadata.(string); ok {
		namespacedName.Name = esName
	}
	if esNamespace, ok := esNamespaceInMetadata.(string); ok {
		namespacedName.Namespace = esNamespace
	}
	return namespacedName, nil
}

// RemoteClustersSettings is used to build a request to update remote clusters.
type RemoteClustersSettings struct {
	PersistentSettings *SettingsGroup `json:"persistent,omitempty"`
}

// SettingsGroup is a group of persistent settings.
type SettingsGroup struct {
	Cluster RemoteClusters `json:"cluster"`
}

// RemoteClusters models the configuration of the remote clusters.
type RemoteClusters struct {
	RemoteClusters map[string]RemoteCluster `json:"remote,omitempty"`
}

// RemoteCluster is the set of seeds to use in a remote cluster setting.
type RemoteCluster struct {
	Seeds []string `json:"seeds"`
}
