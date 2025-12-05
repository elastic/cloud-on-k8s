// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

// APIKey represents an Elasticsearch API key.
type APIKey struct {
	ID       string                 `json:"id,omitempty"`
	Name     string                 `json:"name,omitempty"`
	APIKey   string                 `json:"api_key,omitempty"`
	Encoded  string                 `json:"encoded,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type APIKeyList struct {
	APIKeys []APIKey `json:"api_keys,omitempty"`
}

type APIKeyCreateRequest struct {
	Name string `json:"name,omitempty"`
	APIKeyUpdateRequest
}

type APIKeyUpdateRequest struct {
	APIKey          `json:",inline"`
	Metadata        map[string]any  `json:"metadata,omitempty"`
	RoleDescriptors map[string]Role `json:"role_descriptors,omitempty"`
}

type APIKeyCreateResponse struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
	Encoded string `json:"encoded,omitempty"`
}

type APIKeysInvalidateRequest struct {
	IDs []string `json:"ids,omitempty"`
}

type APIKeysInvalidateResponse struct {
	InvalidatedAPIKeys []string `json:"invalidated_api_keys,omitempty"`
}
