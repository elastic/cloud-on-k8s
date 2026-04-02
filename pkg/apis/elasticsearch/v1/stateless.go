// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"fmt"
	"strings"
)

// ElasticsearchMode specifies the deployment mode for an Elasticsearch cluster.
// +kubebuilder:validation:Enum=stateful;stateless
type ElasticsearchMode string

const (
	// ElasticsearchModeStateful is the traditional Elasticsearch deployment where data is stored on persistent local volumes.
	ElasticsearchModeStateful ElasticsearchMode = "stateful"
	// ElasticsearchModeStateless is the Elasticsearch deployment where data is stored in an external object store.
	ElasticsearchModeStateless ElasticsearchMode = "stateless"
)

// ObjectStoreType specifies the type of object store backend.
// +kubebuilder:validation:Enum=s3;gcs;azure
type ObjectStoreType string

const (
	ObjectStoreTypeS3    ObjectStoreType = "s3"
	ObjectStoreTypeGCS   ObjectStoreType = "gcs"
	ObjectStoreTypeAzure ObjectStoreType = "azure"
)

// ObjectStoreConfig holds the configuration for the external object store used in stateless mode.
type ObjectStoreConfig struct {
	// Type is the object store backend type (s3, gcs, or azure).
	// +kubebuilder:validation:Required
	Type ObjectStoreType `json:"type"`

	// Bucket is the name of the storage bucket.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Bucket string `json:"bucket"`

	// Client is the named client configuration in elasticsearch.yml.
	// Defaults to "default" if not specified.
	// +kubebuilder:validation:Optional
	Client string `json:"client,omitempty"`

	// BasePath is the path prefix within the bucket.
	// +kubebuilder:validation:Optional
	BasePath string `json:"basePath,omitempty"`
}

// StatelessTier represents the functional tier of a NodeSet in stateless mode.
// +kubebuilder:validation:Enum=index;search;master;ml
type StatelessTier string

const (
	IndexTier  StatelessTier = "index"
	SearchTier StatelessTier = "search"
	MasterTier StatelessTier = "master"
	MLTier     StatelessTier = "ml"
)

// knownTierPrefixes maps NodeSet name prefixes to their corresponding stateless tiers.
var knownTierPrefixes = []struct {
	prefix string
	tier   StatelessTier
}{
	{"index", IndexTier},
	{"search", SearchTier},
	{"master", MasterTier},
	{"ml", MLTier},
}

// ResolvedTier returns the tier for a NodeSet, using the explicit tier field if set,
// otherwise inferring from the NodeSet name prefix.
// Returns an error if the tier cannot be resolved.
func (n NodeSet) ResolvedTier() (StatelessTier, error) {
	if n.Tier != "" {
		return n.Tier, nil
	}
	for _, ktp := range knownTierPrefixes {
		if strings.HasPrefix(strings.ToLower(n.Name), ktp.prefix) {
			return ktp.tier, nil
		}
	}
	names := make([]string, len(knownTierPrefixes))
	for i, ktp := range knownTierPrefixes {
		names[i] = ktp.prefix
	}
	return "", fmt.Errorf(
		"cannot infer stateless tier from NodeSet name %q: name must start with one of [%s] or the tier field must be set explicitly",
		n.Name, strings.Join(names, ", "),
	)
}

// IsStateless returns true if this Elasticsearch cluster is configured for stateless mode.
func (es *Elasticsearch) IsStateless() bool {
	return es.Spec.Mode == ElasticsearchModeStateless
}
