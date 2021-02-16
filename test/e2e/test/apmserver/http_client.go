// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

const (
	DefaultReqTimeout = 1 * time.Minute
	backendIntakePath = "/intake/v2/events"
	rumIntakePath     = "/intake/v2/rum/events"
	agentConfigPath   = "/config/v1/agents?service.name=All"
)

// ApmClient is a simple client to use with an Apm Server.
type ApmClient struct {
	client                   *http.Client
	version                  string
	endpoint                 string
	authorizationHeaderValue string
}

func NewApmServerClient(as apmv1.ApmServer, k *test.K8sClient) (*ApmClient, error) {
	var secretTokenSecret v1.Secret
	secretTokenNamespacedName := types.NamespacedName{Namespace: as.Namespace, Name: as.Status.SecretTokenSecretName}
	if err := k.Client.Get(context.Background(), secretTokenNamespacedName, &secretTokenSecret); err != nil {
		return nil, err
	}

	scheme := "http"
	var caCerts []*x509.Certificate
	if as.Spec.HTTP.TLS.Enabled() {
		scheme = "https"
		crts, err := k.GetHTTPCerts(apmserver.Namer, as.Namespace, as.Name)
		if err != nil {
			return nil, err
		}
		caCerts = crts
	}

	inClusterURL := fmt.Sprintf(
		"%s://%s.%s.svc:%d", scheme, as.Status.ExternalService, as.Namespace, apmserver.DefaultHTTPPort,
	)

	client := test.NewHTTPClient(caCerts)

	secretToken, ok := secretTokenSecret.Data[apmserver.SecretTokenKey]
	if !ok {
		return nil, fmt.Errorf("secret token not found in secret: %s", as.Status.SecretTokenSecretName)
	}

	return &ApmClient{
		client:                   client,
		endpoint:                 inClusterURL,
		version:                  as.Spec.Version,
		authorizationHeaderValue: fmt.Sprintf("Bearer %s", secretToken),
	}, nil
}

// doRequest performs an HTTP request using the internal client and automatically adds the required Auth headers
func (c *ApmClient) doRequest(context context.Context, request *http.Request) (*http.Response, error) {
	withContext := request.WithContext(context)

	// inject the authorization (secret token)
	request.Header.Set("Authorization", c.authorizationHeaderValue)

	// allow json content
	request.Header.Set("Accept", "application/json")

	return c.client.Do(withContext)
}

// request performs a new http request
//
// if requestObj is not nil, it's marshalled as JSON and used as the request body
// if responseObj is not nil, it should be a pointer to an struct. the response body will be unmarshalled from JSON
// into this struct.
func (c *ApmClient) request(
	ctx context.Context,
	method string,
	pathWithQuery string,
	headers http.Header,
	requestObj,
	responseObj interface{},
) error {
	var body io.Reader = http.NoBody

	if requestObj != nil {
		outData, err := json.Marshal(requestObj)
		if err != nil {
			return err
		}
		body = bytes.NewBuffer(outData)
	}

	request, err := http.NewRequest(method, stringsutil.Concat(c.endpoint, pathWithQuery), body)
	if err != nil {
		return err
	}

	request.Header = headers

	resp, err := c.doRequest(ctx, request)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if responseObj != nil {
		if err := json.NewDecoder(resp.Body).Decode(responseObj); err != nil {
			return err
		}
	}

	return nil
}

// ApmServerInfo is a partial encoding of the Server Info response.
// See https://www.elastic.co/guide/en/apm/server/current/server-info.html for more details.
type ApmServerInfo struct {
	// Version is the version of the Apm Server
	Version string `json:"version"`
}

type ApmServerInfo6 struct {
	// OK contains the ApmServerInfo
	OK ApmServerInfo `json:"ok"`
}

// ServerInfo requests the Server Information API
func (c *ApmClient) ServerInfo(ctx context.Context) (*ApmServerInfo, error) {
	requester := func(responseObj interface{}) error {
		if err := c.request(ctx, http.MethodGet, "", http.Header{
			"Accept": []string{"application/json"},
		}, nil, &responseObj); err != nil {
			return err
		}
		return nil
	}

	if strings.HasPrefix(c.version, "6") {
		var serverInfo ApmServerInfo6
		return &serverInfo.OK, requester(&serverInfo)
	}
	var serverInfo ApmServerInfo
	return &serverInfo, requester(&serverInfo)
}

// EventsErrorResponse is the error response format used by the Events API.
type EventsErrorResponse struct {
	// Errors describes the events that had errors.
	Errors []EventsError `json:"errors,omitempty"`
	// Accepted is the number of accepted events.
	Accepted int `json:"accepted,omitempty"`
}

// EventsError describes a single error event
type EventsError struct {
	// Message is the error
	Message string `json:"message,omitempty"`
	// Document is the document/event that is the source of the error.
	Document string `json:"document,omitempty"`
}

// IntakeV2Events exposes the Events API.
// In the happy case, this will return nil, nil, indicating all events were accepted.
// See https://www.elastic.co/guide/en/apm/server/current/events-api.html for more details.
func (c *ApmClient) IntakeV2Events(ctx context.Context, rum bool, payload []byte) (*EventsErrorResponse, error) {
	var eventsErrorResponse EventsErrorResponse

	path := backendIntakePath
	if rum {
		path = rumIntakePath
	}
	request, err := http.NewRequest(
		http.MethodPost,
		stringsutil.Concat(c.endpoint, path),
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return nil, err
	}

	// set the content type to the newline-delimited JSON type:
	request.Header.Set("Content-Type", "application/x-ndjson")

	resp, err := c.doRequest(ctx, request)
	if err != nil {
		return nil, err
	}

	defer func() {
		request.Body.Close()
		resp.Body.Close()
	}()

	// if it was accepted, there were no errors
	if resp.StatusCode == http.StatusAccepted {
		return nil, nil
	}

	if err := json.NewDecoder(resp.Body).Decode(&eventsErrorResponse); err != nil {
		return nil, err
	}

	return &eventsErrorResponse, err
}

// AgentConfig describes an agent configuration
type AgentConfig struct {
	CaptureBody           string `json:"capture_body,omitempty"`
	TransactionMaxSpans   string `json:"transaction_max_spans,omitempty"`
	TransactionSampleRate string `json:"transaction_sample_rate,omitempty"`
	Error                 string `json:"error,omitempty"`
}

// AgentsDefaultConfig returns the default Agent configuration
func (c *ApmClient) AgentsDefaultConfig(ctx context.Context) (AgentConfig, error) {
	var agentConfig AgentConfig
	request, err := http.NewRequest(
		http.MethodGet,
		stringsutil.Concat(c.endpoint, agentConfigPath),
		nil,
	)
	if err != nil {
		return agentConfig, err
	}

	resp, err := c.doRequest(ctx, request)
	if err != nil {
		return agentConfig, err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&agentConfig); err != nil {
		return agentConfig, err
	}

	return agentConfig, nil
}
