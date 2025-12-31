// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"context"
	"fmt"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

type ServiceAccountCredential struct {
	NodesCredentials NodesCredentials `json:"nodes_credentials"`
}

type NodesCredentials struct {
	FileTokens map[string]FileToken `json:"file_tokens"`
}

type FileToken struct {
	Nodes []string `json:"nodes"`
}

// Nodes returns the list of nodes which are referenced in the API response.
func (s *ServiceAccountCredential) Nodes() set.StringSet {
	result := set.Make()
	for _, fileToken := range s.NodesCredentials.FileTokens {
		for _, nodeName := range fileToken.Nodes {
			result.Add(nodeName)
		}
	}
	return result
}

type SecurityClient interface {

	// GetServiceAccountCredentials returns the service account credentials from the /_security/service API
	GetServiceAccountCredentials(ctx context.Context, namespacedService string) (ServiceAccountCredential, error)
	// GetAPIKeysByName returns the API keys by name from the /_security/api_key API
	GetAPIKeysByName(ctx context.Context, name string) (APIKeyList, error)
	// CreateAPIKey creates a new API key from the /_security/api_key API
	CreateAPIKey(ctx context.Context, request APIKeyCreateRequest) (APIKeyCreateResponse, error)
	// InvalidateAPIKeys invalidates one or more API keys by their IDs from the /_security/api_key API
	InvalidateAPIKeys(ctx context.Context, request APIKeysInvalidateRequest) (APIKeysInvalidateResponse, error)
}

func (c *clientV6) GetServiceAccountCredentials(_ context.Context, _ string) (ServiceAccountCredential, error) {
	return ServiceAccountCredential{}, errNotSupportedInEs6x
}

func (c *clientV7) GetServiceAccountCredentials(ctx context.Context, namespacedService string) (ServiceAccountCredential, error) {
	var serviceAccountCredential ServiceAccountCredential
	path := fmt.Sprintf("/_security/service/%s/credential", namespacedService)
	if err := c.get(ctx, path, &serviceAccountCredential); err != nil {
		return serviceAccountCredential, err
	}
	return serviceAccountCredential, nil
}

func (c *clientV6) GetAPIKeysByName(ctx context.Context, name string) (APIKeyList, error) {
	return APIKeyList{}, errNotSupportedInEs6x
}

func (c *clientV7) GetAPIKeysByName(ctx context.Context, name string) (APIKeyList, error) {
	var apiKeys APIKeyList
	path := fmt.Sprintf("/_security/api_key?name=%s", name)
	if err := c.get(ctx, path, &apiKeys); err != nil {
		return apiKeys, err
	}
	// active_only=true was added in 8.10, but since we support all versions of V8 and Elasticsearch
	// returns a 400 error if active_only=true is used we can't use it. We must filter
	// out inactive api keys manually to have the same behavior across all versions Of Elasticsearch.
	activeAPIKeys := make([]APIKey, 0, len(apiKeys.APIKeys))
	for _, apiKey := range apiKeys.APIKeys {
		if apiKey.isActive() {
			activeAPIKeys = append(activeAPIKeys, apiKey)
		}
	}
	return APIKeyList{APIKeys: activeAPIKeys}, nil
}

func (c *clientV6) CreateAPIKey(ctx context.Context, request APIKeyCreateRequest) (APIKeyCreateResponse, error) {
	return APIKeyCreateResponse{}, errNotSupportedInEs6x
}

func (c *clientV7) CreateAPIKey(ctx context.Context, request APIKeyCreateRequest) (APIKeyCreateResponse, error) {
	var apiKey APIKeyCreateResponse
	path := "/_security/api_key"
	if err := c.post(ctx, path, request, &apiKey); err != nil {
		return apiKey, err
	}
	return apiKey, nil
}

func (c *clientV6) InvalidateAPIKeys(ctx context.Context, request APIKeysInvalidateRequest) (APIKeysInvalidateResponse, error) {
	return APIKeysInvalidateResponse{}, errNotSupportedInEs6x
}

func (c *clientV7) InvalidateAPIKeys(ctx context.Context, request APIKeysInvalidateRequest) (APIKeysInvalidateResponse, error) {
	path := "/_security/api_key"
	var response APIKeysInvalidateResponse
	if err := c.deleteWithObjects(ctx, path, request, &response); err != nil {
		return response, err
	}
	return response, nil
}
