// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"context"
	"fmt"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
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
