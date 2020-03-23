// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

type baseClient struct {
	User     BasicAuth
	HTTP     *http.Client
	Endpoint string
	caCerts  []*x509.Certificate
}

// Close idle connections in the underlying http client.
// Should be called once this client is not used anymore.
func (c *baseClient) Close() {
	if c.HTTP != nil {
		// When the http transport goes out of scope, the underlying goroutines responsible
		// for handling keep-alive connections are not closed automatically.
		// Since this client gets recreated frequently we would effectively be leaking goroutines.
		// Let's make sure this does not happen by closing idle connections.
		c.HTTP.CloseIdleConnections()
	}
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

func (c *baseClient) doRequest(context context.Context, request *http.Request) (*http.Response, error) {
	withContext := request.WithContext(context)
	withContext.Header.Set("Content-Type", "application/json; charset=utf-8")

	if c.User != (BasicAuth{}) {
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

func versioned(b *baseClient, v version.Version) Client {
	v6 := clientV6{
		baseClient: *b,
	}
	switch v.Major {
	case 7:
		return &clientV7{
			clientV6: v6,
		}
	case 8:
		return &clientV8{
			clientV7: clientV7{clientV6: v6},
		}
	default:
		return &v6
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
