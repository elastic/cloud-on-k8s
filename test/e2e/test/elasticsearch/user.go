// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
)

// HasPrivilegesResponse is the response body from /_security/user/_has_privileges.
type HasPrivilegesResponse struct {
	Username        string                     `json:"username"`
	HasAllRequested bool                       `json:"has_all_requested"`
	Cluster         map[string]bool            `json:"cluster"`
	Index           map[string]map[string]bool `json:"index"`
}

// HasPrivilegesAs calls /_security/user/_has_privileges impersonating runAsUser via the
// es-security-runas-user header. The esClient must authenticate as a user that holds the
// run_as privilege for runAsUser (e.g. the elastic superuser). body is a JSON privileges
// request object, e.g. `{"cluster":["monitor","manage"]}`.
func HasPrivilegesAs(ctx context.Context, esClient client.Client, runAsUser, body string) (HasPrivilegesResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/_security/user/_has_privileges", strings.NewReader(body))
	if err != nil {
		return HasPrivilegesResponse{}, err
	}

	respBytes, err := runAs(ctx, req, esClient, runAsUser)
	if err != nil {
		return HasPrivilegesResponse{}, err
	}

	var result HasPrivilegesResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return HasPrivilegesResponse{}, fmt.Errorf("failed to parse _has_privileges response: %w, body: %s", err, string(respBytes))
	}
	return result, nil
}

// AuthenticateResponse is the response body from /_security/_authenticate.
type AuthenticateResponse struct {
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
	Enabled  bool     `json:"enabled"`
}

func Authenticate(ctx context.Context, esClient client.Client, runAsUser string) (AuthenticateResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/_security/_authenticate", http.NoBody)
	if err != nil {
		return AuthenticateResponse{}, err
	}

	respBytes, err := runAs(ctx, req, esClient, runAsUser)
	if err != nil {
		return AuthenticateResponse{}, err
	}

	var result AuthenticateResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return AuthenticateResponse{}, fmt.Errorf("failed to parse _authenticate response: %w, body: %s", err, string(respBytes))
	}
	return result, nil
}

func runAs(ctx context.Context, req *http.Request, esClient client.Client, runAsUser string) ([]byte, error) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("es-security-runas-user", runAsUser)
	resp, err := esClient.Request(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
