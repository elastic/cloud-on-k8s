// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"

	"github.com/hashicorp/go-multierror"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

var log = ulog.Log.WithName("elasticsearch-client")

type baseClient struct {
	User     BasicAuth
	HTTP     *http.Client
	Endpoint string
	es       types.NamespacedName
	caCerts  []*x509.Certificate
	version  version.Version
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

	log.V(1).Info(
		"Elasticsearch HTTP request",
		"method", request.Method,
		"url", request.URL.Redacted(),
		"namespace", c.es.Namespace,
		"es_name", c.es.Name,
	)
	response, err := c.HTTP.Do(withContext)
	if err != nil {
		return response, newDecoratedHTTPError(request, err)
	}

	// Check HTTP code in Elasticsearch response.
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response, newDecoratedHTTPError(request, newAPIError(response))
	}

	return response, nil
}

func (c *baseClient) get(ctx context.Context, pathWithQuery string, out interface{}) error {
	return c.request(ctx, http.MethodGet, pathWithQuery, nil, out, nil)
}

func (c *baseClient) put(ctx context.Context, pathWithQuery string, in, out interface{}) error { //nolint:unparam
	return c.request(ctx, http.MethodPut, pathWithQuery, in, out, nil)
}

func (c *baseClient) post(ctx context.Context, pathWithQuery string, in, out interface{}) error {
	return c.request(ctx, http.MethodPost, pathWithQuery, in, out, nil)
}

func (c *baseClient) delete(ctx context.Context, pathWithQuery string) error {
	return c.request(ctx, http.MethodDelete, pathWithQuery, nil, nil, nil)
}

// request performs a new http request
//
// if requestObj is not nil, it's marshalled as JSON and used as the request body
// if responseObj is not nil, it should be a pointer to an struct. The response body will be unmarshalled from JSON
// into this struct if the status code of the response is 2xx or if the (optional) given skipErrFunc function returns true.
func (c *baseClient) request(
	ctx context.Context,
	method string,
	pathWithQuery string,
	requestObj,
	responseObj interface{},
	skipErrFunc func(error) bool,
) error {
	var body io.Reader = http.NoBody
	if requestObj != nil {
		outData, err := json.Marshal(requestObj)
		if err != nil {
			return err
		}
		body = bytes.NewBuffer(outData)
	}

	request, err := http.NewRequest(method, stringsutil.Concat(c.Endpoint, pathWithQuery), body) //nolint:noctx
	if err != nil {
		return err
	}

	var skippedErr error
	resp, err := c.doRequest(ctx, request)
	if skipErrFunc != nil && skipErrFunc(err) {
		skippedErr = err
		err = nil
	}
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if responseObj != nil {
		if err := json.NewDecoder(resp.Body).Decode(responseObj); err != nil {
			if skippedErr != nil {
				err = multierror.Append(err, skippedErr)
			}
			return err
		}
	}

	return nil
}

func versioned(b *baseClient, v version.Version) Client {
	b.version = v
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
