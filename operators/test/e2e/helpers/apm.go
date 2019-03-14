// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	apmtype "github.com/elastic/k8s-operators/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/apmserver"
	"github.com/elastic/k8s-operators/operators/pkg/dev/portforward"
	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
	"io"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
)

// ApmClient is a simple client to use with an Apm Server.
type ApmClient struct {
	client                   *http.Client
	endpoint                 string
	authorizationHeaderValue string
}

func NewApmServerClient(as apmtype.ApmServer, k *K8sHelper) (*ApmClient, error) {
	var secretTokenSecret v1.Secret
	secretTokenNamespacedName := types.NamespacedName{Namespace: as.Namespace, Name: as.Status.SecretTokenSecretName}
	if err := k.Client.Get(secretTokenNamespacedName, &secretTokenSecret); err != nil {
		return nil, err
	}

	inClusterURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:8200", as.Status.ExternalService, as.Namespace)
	var dialer net.Dialer
	if *autoPortForward {
		dialer = portforward.NewForwardingDialer()
	}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
		},
	}

	secretToken, ok := secretTokenSecret.Data[apmserver.SecretTokenKey]
	if !ok {
		return nil, fmt.Errorf("secret token not found in secret: %s", as.Status.SecretTokenSecretName)
	}

	return &ApmClient{
		client:                   client,
		endpoint:                 inClusterURL,
		authorizationHeaderValue: fmt.Sprintf("Bearer %s", secretToken),
	}, nil
}

// doRequest performs an HTTP request using the internal client and automatically adds the required Auth headers
func (c *ApmClient) doRequest(context context.Context, request *http.Request) (*http.Response, error) {
	withContext := request.WithContext(context)

	// inject the authorization (secret token)
	request.Header.Set("Authorization", c.authorizationHeaderValue)

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

// ServerInfo requests the Server Information API
func (c *ApmClient) ServerInfo(ctx context.Context) (*ApmServerInfo, error) {
	var serverInfo ApmServerInfo

	if err := c.request(ctx, http.MethodGet, "", nil, &serverInfo); err != nil {
		return nil, err
	}

	return &serverInfo, nil
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
func (c *ApmClient) IntakeV2Events(ctx context.Context, payload []byte) (*EventsErrorResponse, error) {
	var eventsErrorResponse EventsErrorResponse

	request, err := http.NewRequest(
		http.MethodPost,
		stringsutil.Concat(c.endpoint, "/intake/v2/events"),
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

	defer request.Body.Close()

	// if it was accepted, there were no errors
	if resp.StatusCode == http.StatusAccepted {
		return nil, nil
	}

	if err := json.NewDecoder(resp.Body).Decode(eventsErrorResponse); err != nil {
		return nil, err
	}

	return &eventsErrorResponse, err
}
