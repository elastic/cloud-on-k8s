// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
)

type baseClient struct {
	User     UserAuth
	HTTP     *http.Client
	Endpoint string
	caCerts  []*x509.Certificate
}

func (c *baseClient) equal(c2 *baseClient) bool {
	// handle nil case
	if c2 == nil && c != nil {
		return false
	}
	// compare ca certs
	if len(c.caCerts) != len(c2.caCerts) {
		return false
	}
	for i := range c.caCerts {
		if !c.caCerts[i].Equal(c2.caCerts[i]) {
			return false
		}
	}
	// compare endpoint and user creds
	return c.Endpoint == c2.Endpoint &&
		c.User == c2.User
}

// NewElasticsearchClient creates a new client for the target cluster.
//
// If dialer is not nil, it will be used to create new TCP connections
func NewElasticsearchClient(dialer net.Dialer, esURL string, esUser UserAuth, v version.Version, caCerts []*x509.Certificate) Client {
	certPool := x509.NewCertPool()
	for _, c := range caCerts {
		certPool.AddCert(c)
	}

	transportConfig := http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: certPool,
		},
	}

	// use the custom dialer if provided
	if dialer != nil {
		transportConfig.DialContext = dialer.DialContext
	}
	base := &baseClient{
		Endpoint: esURL,
		User:     esUser,
		caCerts:  caCerts,
		HTTP: &http.Client{
			Transport: &transportConfig,
		},
	}
	return versioned(base, v)
}

func versioned(b *baseClient, v version.Version) Client {
	v6 := clientV6{
		baseClient: *b,
	}
	switch v.Major {
	case 7:
		return &clientV7{
			clientV6: v6,
		}
	default:
		return &v6
	}
}

// APIError is a non 2xx response from the Elasticsearch API
type APIError struct {
	response *http.Response
}

// Error() implements the error interface.
func (e *APIError) Error() string {
	defer e.response.Body.Close()
	reason := "unknown"
	// Elasticsearch has a detailed error message in the response body
	var errMsg ErrorResponse
	err := json.NewDecoder(e.response.Body).Decode(&errMsg)
	if err == nil {
		reason = errMsg.Error.Reason
	}
	return fmt.Sprintf("%s: %s", e.response.Status, reason)
}

// IsNotFound checks whether the error was a HTTP 404 error.
func IsNotFound(err error) bool {
	switch err := err.(type) {
	case *APIError:
		return err.response.StatusCode == http.StatusNotFound
	default:
		return false
	}
}

func checkError(response *http.Response) error {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &APIError{
			response: response,
		}
	}
	return nil
}

func (c *baseClient) doRequest(context context.Context, request *http.Request) (*http.Response, error) {
	withContext := request.WithContext(context)
	withContext.Header.Set("Content-Type", "application/json; charset=utf-8")

	if c.User != (UserAuth{}) {
		withContext.SetBasicAuth(c.User.Name, c.User.Password)
	}

	response, err := c.HTTP.Do(withContext)
	if err != nil {
		return response, err
	}
	err = checkError(response)
	return response, err
}

func (c *baseClient) get(ctx context.Context, pathWithQuery string, out interface{}) error {
	return c.request(ctx, http.MethodGet, pathWithQuery, nil, out)
}

func (c *baseClient) put(ctx context.Context, pathWithQuery string, in, out interface{}) error {
	return c.request(ctx, http.MethodPut, pathWithQuery, in, out)
}

func (c *baseClient) post(ctx context.Context, pathWithQuery string, in, out interface{}) error {
	return c.request(ctx, http.MethodPost, pathWithQuery, in, out)
}

func (c *baseClient) delete(ctx context.Context, pathWithQuery string, in, out interface{}) error {
	return c.request(ctx, http.MethodDelete, pathWithQuery, in, out)
}

// request performs a new http request
//
// if requestObj is not nil, it's marshalled as JSON and used as the request body
// if responseObj is not nil, it should be a pointer to an struct. the response body will be unmarshalled from JSON
// into this struct.
func (c *baseClient) request(
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

	request, err := http.NewRequest(method, stringsutil.Concat(c.Endpoint, pathWithQuery), body)
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
