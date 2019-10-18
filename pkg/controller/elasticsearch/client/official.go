// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	elasticsearch6 "github.com/elastic/go-elasticsearch/v6"
	esapi6 "github.com/elastic/go-elasticsearch/v6/esapi"
	elasticsearch7 "github.com/elastic/go-elasticsearch/v7"
	esapi7 "github.com/elastic/go-elasticsearch/v7/esapi"
	elasticsearch8 "github.com/elastic/go-elasticsearch/v8"
	esapi8 "github.com/elastic/go-elasticsearch/v8/esapi"
)

// elasticsearchClients captures the information needed to interact with an Elasticsearch cluster via HTTP for any
// supported Elasticsearch version
type elasticsearchClients struct {
	es6 *elasticsearch6.Client
	es7 *elasticsearch7.Client
	es8 *elasticsearch8.Client
}

func newElasticsearchClients(
	v version.Version,
	url string,
	auth UserAuth,
	transport http.RoundTripper,
) (*elasticsearchClients, error) {
	switch v.Major {
	case 6:
		es6, err := elasticsearch6.NewClient(elasticsearch6.Config{
			Addresses: []string{url},
			Username:  auth.Name,
			Password:  auth.Password,
			Transport: transport,
			Logger:    &esLogger{},
		})
		if err != nil {
			return nil, err
		}
		return &elasticsearchClients{
			es6: es6,
		}, nil
	case 7:
		es7, err := elasticsearch7.NewClient(elasticsearch7.Config{
			Addresses: []string{url},
			Username:  auth.Name,
			Password:  auth.Password,
			Transport: transport,
			Logger:    &esLogger{},
		})
		if err != nil {
			return nil, err
		}
		return &elasticsearchClients{
			es7: es7,
		}, nil
	default:
		es8, err := elasticsearch8.NewClient(elasticsearch8.Config{
			Addresses: []string{url},
			Username:  auth.Name,
			Password:  auth.Password,
			Transport: transport,
			Logger:    &esLogger{},
		})
		if err != nil {
			return nil, err
		}
		return &elasticsearchClients{
			es8: es8,
		}, nil
	}
}

// versionedRequestWithUnversionedResponse is used to implement requests to Elasticsearch that varies depending on
// the target Elasticsearch version, but the response is on a common format between them.
type versionedRequestWithUnversionedResponse struct {
	ES6 func(es *elasticsearch6.Client) (*esapi6.Response, error)
	ES7 func(es *elasticsearch7.Client) (*esapi7.Response, error)
	ES8 func(es *elasticsearch8.Client) (*esapi8.Response, error)

	// ResponseHandler is called with the generic response from Elasticsearch.
	ResponseHandler func(response *Response) error
}

// defaultUnversionedResponseHandler returns Elasticsearch API-level error responses as errors.
var defaultUnversionedResponseHandler = func(response *Response) error {
	if response.IsError() {
		return response.ESError()
	}
	return nil
}

// doVersionedRequestWithUnversionedResponse performs a request whose request may vary by version.
// If no response handler is provided, defaultUnversionedResponseHandler will be used.
// This method automatically closes the response body reader before returning.
func (c *elasticsearchClients) doVersionedRequestWithUnversionedResponse(
	request versionedRequestWithUnversionedResponse,
) error {
	if request.ResponseHandler == nil {
		request.ResponseHandler = defaultUnversionedResponseHandler
	}

	var response *Response
	switch {
	case c.es6 != nil && request.ES6 != nil:
		res, err := request.ES6(c.es6)
		if err != nil {
			return err
		}
		response = newResponseFrom6(res)
	case c.es7 != nil && request.ES7 != nil:
		res, err := request.ES7(c.es7)
		if err != nil {
			return err
		}
		response = newResponseFrom7(res)
	case c.es8 != nil && request.ES8 != nil:
		res, err := request.ES8(c.es8)
		if err != nil {
			return err
		}
		response = newResponseFrom8(res)
	default:
		// no requests specified for the current version, nothing to do
		return nil
	}

	if response.Body != nil {
		defer response.Body.Close()
	}
	return request.ResponseHandler(response)
}

// perform performs the request using the underlying Elasticsearch clients transport
func (c *elasticsearchClients) perform(request *http.Request) (*http.Response, error) {
	switch {
	case c.es6 != nil:
		return c.es6.Transport.Perform(request)
	case c.es7 != nil:
		return c.es7.Transport.Perform(request)
	default:
		return c.es8.Transport.Perform(request)
	}
}

// customRequest7 performs a custom request to Elasticsearch v7
func customRequest7(ctx context.Context, es *elasticsearch7.Client, request *http.Request) (*esapi7.Response, error) {
	request = request.WithContext(ctx)

	// nolint:bodyclose
	httpRes, err := es.Perform(request)
	if err != nil {
		return nil, err
	}

	res := &esapi7.Response{
		StatusCode: httpRes.StatusCode,
		Header:     httpRes.Header,
		Body:       httpRes.Body,
	}

	return res, nil
}

// customRequest8 performs a custom request to Elasticsearch v8
func customRequest8(ctx context.Context, es *elasticsearch8.Client, request *http.Request) (*esapi8.Response, error) {
	request = request.WithContext(ctx)

	// nolint:bodyclose
	httpRes, err := es.Perform(request)
	if err != nil {
		return nil, err
	}

	res := &esapi8.Response{
		StatusCode: httpRes.StatusCode,
		Header:     httpRes.Header,
		Body:       httpRes.Body,
	}

	return res, nil
}

// Response
type Response struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser

	// esErrorOnce is used to ensure we only decode an error once from the body
	esErrorOnce sync.Once
	// esError stores the cached decoded error from the body
	esError *officialAPIError
}

func newResponseFrom6(res *esapi6.Response) *Response {
	return &Response{
		StatusCode: res.StatusCode,
		Header:     res.Header,
		Body:       res.Body,
	}
}

func newResponseFrom7(res *esapi7.Response) *Response {
	return &Response{
		StatusCode: res.StatusCode,
		Header:     res.Header,
		Body:       res.Body,
	}
}

func newResponseFrom8(res *esapi8.Response) *Response {
	return &Response{
		StatusCode: res.StatusCode,
		Header:     res.Header,
		Body:       res.Body,
	}
}

// IsError is true if the HTTP status code is outside 2xx
func (r *Response) IsError() bool {
	return r.StatusCode < 200 || r.StatusCode > 299
}

// ESError returns the Elasticsearch API response as an error
func (r *Response) ESError() error {
	r.esErrorOnce.Do(func() {
		r.esError = newOfficialAPIError(r)
	})
	return r.esError
}

// officialAPIError is a non 2xx response from the Elasticsearch API
type officialAPIError struct {
	// statusCode is the HTTP status code from the HTTP response
	statusCode int
	// errorResponse is the structured ErrorResponse parsed from the HTTP response body
	errorResponse *ErrorResponse
	// decodeError is the JSON decoding error while decoding the ErrorResponse
	decodeError error
}

// newOfficialAPIError parses the Response into an officialAPIError.
func newOfficialAPIError(r *Response) *officialAPIError {
	var errorResponse ErrorResponse

	decodeError := json.NewDecoder(r.Body).Decode(&errorResponse)

	return &officialAPIError{
		statusCode:    r.StatusCode,
		errorResponse: &errorResponse,
		decodeError:   decodeError,
	}
}

func (e *officialAPIError) Error() string {
	b := strings.Builder{}

	b.WriteString(strconv.Itoa(e.statusCode))
	b.WriteString(": ")

	reason := "unknown"
	if e.errorResponse.Error.Reason != "" {
		reason = e.errorResponse.Error.Reason
	}
	b.WriteString(reason)

	if e.decodeError != nil {
		b.WriteString(" (decode error: ")
		b.WriteString(e.decodeError.Error())
		b.WriteString(")")
	}

	return b.String()
}

// esErrorOrDecodeJSON returns the API error in the response if it is an error or decodes the response body into the
// provided value.
func esErrorOrDecodeJSON(v interface{}) func(response *Response) error {
	return func(response *Response) error {
		if response.IsError() {
			return response.ESError()
		}

		return json.NewDecoder(response.Body).Decode(v)
	}
}
