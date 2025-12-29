// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apikey

const (
	// MetadataKeyManagedBy identifies API keys managed by ECK
	MetadataKeyManagedBy = "elasticsearch.k8s.elastic.co/managed-by"
	// MetadataValueECK is the value indicating ECK management
	MetadataValueECK = "eck"
	// MetadataKeyConfigHash stores the config hash for change detection
	MetadataKeyConfigHash = "elasticsearch.k8s.elastic.co/config-hash"
	// MetadataKeyESName stores the Elasticsearch cluster name
	MetadataKeyESName = "elasticsearch.k8s.elastic.co/name"
	// MetadataKeyESNamespace stores the Elasticsearch cluster namespace
	MetadataKeyESNamespace = "elasticsearch.k8s.elastic.co/namespace"
)

// IsManagedByECK checks if an API key's metadata indicates ECK management
func IsManagedByECK(metadata map[string]any) bool {
	if metadata == nil {
		return false
	}
	managedBy, ok := metadata[MetadataKeyManagedBy].(string)
	return ok && managedBy == MetadataValueECK
}

// NeedsUpdate checks if the API key needs updating based on config hash
func NeedsUpdate(metadata map[string]any, expectedHash string) bool {
	if metadata == nil {
		return true
	}
	currentHash, ok := metadata[MetadataKeyConfigHash].(string)
	return !ok || currentHash != expectedHash
}
