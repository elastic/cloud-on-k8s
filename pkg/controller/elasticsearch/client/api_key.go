// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import "time"

// APIKey represents an Elasticsearch API key.
type APIKey struct {
	APIKey      string         `json:"api_key,omitempty"`
	ID          string         `json:"id,omitempty"`
	Encoded     string         `json:"encoded,omitempty"`
	Expiration  *int64         `json:"expiration,omitempty"`
	Invalidated bool           `json:"invalidated,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Name        string         `json:"name,omitempty"`
}

// isExpired returns whether the API key has expired.
func (k *APIKey) isExpired() bool {
	if k.Expiration == nil {
		return false
	}
	expirationTime := time.Unix(0, *k.Expiration*int64(time.Millisecond))
	return time.Now().After(expirationTime)
}

// isInvalidated returns whether the API key has been invalidated.
func (k *APIKey) isInvalidated() bool {
	return k.Invalidated
}

// isActive returns whether the API key is both not expired and not invalidated.
func (k *APIKey) isActive() bool {
	return !k.isExpired() && !k.isInvalidated()
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
