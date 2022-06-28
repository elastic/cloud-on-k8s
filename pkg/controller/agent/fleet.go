// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

const FleetTokenAnnotation = "fleet.eck.k8s.elastic.co/token"

// TODO common kibana/http client?
type APIError struct {
	StatusCode int
	msg        string
}

func (e *APIError) Error() string {
	return e.msg
}

type EnrollmentAPIKeyResult struct {
	Item EnrollmentAPIKey `json:"item"`
}

type EnrollmentAPIKey struct {
	ID       string `json:"id,omitempty"`
	Active   bool   `json:"active,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	PolicyID string `json:"policy_id,omitempty"`
}

type AgentPolicyList struct {
	Items []AgentPolicy `json:"items"`
}

type AgentPolicy struct {
	ID                   string `json:"id"`
	IsDefault            bool   `json:"is_default"`
	IsDefaultFleetServer bool   `json:"is_default_fleet_server"`
}

type fleetAPI struct {
	client        *http.Client
	endpoint      string
	username      string
	password      string
	kibanaVersion string
	log           logr.Logger
}

func newFleetAPI(dialer net.Dialer, settings connectionSettings, logger logr.Logger) fleetAPI {
	return fleetAPI{
		client:        common.HTTPClient(dialer, settings.caCerts, 60*time.Second),
		kibanaVersion: settings.version,
		endpoint:      settings.host,
		username:      settings.credentials.Username,
		password:      settings.credentials.Password,
		log:           logger,
	}
}

func (f fleetAPI) request(
	ctx context.Context,
	method string,
	pathWithQuery string,
	requestObj,
	responseObj interface{}) error {

	var body io.Reader = http.NoBody
	if requestObj != nil {
		outData, err := json.Marshal(requestObj)
		if err != nil {
			return err
		}
		body = bytes.NewBuffer(outData)
	}

	request, err := http.NewRequestWithContext(ctx, method, stringsutil.Concat(f.endpoint, "/api/fleet/", pathWithQuery), body)
	if err != nil {
		return err
	}

	// Sets headers allowing ES to distinguish between deprecated APIs used internally and by the user
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	request.Header.Set(common.InternalProductRequestHeaderKey, common.InternalProductRequestHeaderValue)
	request.Header.Set("kbn-xsrf", "true")
	request.SetBasicAuth(f.username, f.password)

	f.log.V(1).Info(
		"Fleet API HTTP request",
		"method", request.Method,
		"url", request.URL.Redacted(),
	)

	resp, err := f.client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &APIError{
			StatusCode: resp.StatusCode,
			msg:        fmt.Sprintf("failed to request %s, status is %d)", request.URL.Redacted(), resp.StatusCode),
		}
	}
	if responseObj != nil {
		if err := json.NewDecoder(resp.Body).Decode(responseObj); err != nil {
			return err
		}
	}

	return nil

}

func (f fleetAPI) CreateEnrollmentAPIKey(ctx context.Context, policyID string) (EnrollmentAPIKey, error) {
	path := "enrollment_api_keys"
	if strings.HasPrefix(f.kibanaVersion, "7") {
		path = strings.Replace(path, "_", "-", -1)
	}
	var response EnrollmentAPIKeyResult
	err := f.request(ctx, http.MethodPost, path, EnrollmentAPIKey{PolicyID: policyID}, &response)
	return response.Item, err
}

func (f fleetAPI) GetEnrollmentAPIKey(ctx context.Context, keyID string) (EnrollmentAPIKey, error) {
	path := "enrollment_api_keys"
	if strings.HasPrefix(f.kibanaVersion, "7") {
		path = strings.Replace(path, "_", "-", -1)
	}
	var response EnrollmentAPIKeyResult
	err := f.request(ctx, http.MethodGet, fmt.Sprintf("%s/%s", path, keyID), nil, &response)
	return response.Item, err
}

func (f fleetAPI) ListAgentPolicies(ctx context.Context) (AgentPolicyList, error) {
	var list AgentPolicyList
	err := f.request(ctx, http.MethodGet, "agent_policies", nil, &list)
	return list, err
}

func (f fleetAPI) DefaultFleetServerPolicyID(ctx context.Context) (string, error) {
	policies, err := f.ListAgentPolicies(ctx)
	if err != nil {
		return "", err
	}
	for _, p := range policies.Items {
		if p.IsDefaultFleetServer {
			return p.ID, nil
		}
	}
	return "", errors.New("no default fleet server policy found")
}

func (f fleetAPI) DefaultAgentPolicyID(ctx context.Context) (string, error) {
	policies, err := f.ListAgentPolicies(ctx)
	if err != nil {
		return "", err
	}
	for _, p := range policies.Items {
		if p.IsDefault {
			return p.ID, nil
		}
	}
	return "", errors.New("no default agent policy found")
}

func reconcileEnrollmentToken(
	ctx context.Context,
	agent agentv1alpha1.Agent,
	client k8s.Client,
	api fleetAPI,
) (string, error) {
	tokenName, exists := agent.Annotations[FleetTokenAnnotation]
	policyID, err := reconcilePolicyID(ctx, agent, api)
	if err != nil {
		return "", err
	}
	if exists {
		key, err := api.GetEnrollmentAPIKey(ctx, tokenName)
		if err != nil {
			return "", err
		}
		if key.Active {
			return key.APIKey, nil
		}
	}
	key, err := api.CreateEnrollmentAPIKey(ctx, policyID)
	if err != nil {
		return "", err
	}
	// TODO  this creates conflicts solve on top level
	agent.Annotations[FleetTokenAnnotation] = key.ID
	err = client.Update(ctx, &agent)
	if err != nil {
		return "", err
	}
	// TODO failed update creates dangling API key
	return key.APIKey, nil
}

func reconcilePolicyID(ctx context.Context, agent agentv1alpha1.Agent, api fleetAPI) (string, error) {
	/*if agent.Spec.PolicyID != "" {
		return agent.Spec.PolicyID
	}*/
	if agent.Spec.FleetServerEnabled {
		return api.DefaultFleetServerPolicyID(ctx)
	}
	return api.DefaultAgentPolicyID(ctx)

}
