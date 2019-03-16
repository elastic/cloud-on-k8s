// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"io/ioutil"
	"net/http"
	"strings"

	ver "github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
)

type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func NewMockClient(v ver.Version, fn RoundTripFunc) Client {
	return Client{
		HTTP: &http.Client{
			Transport: RoundTripFunc(fn),
		},
		Endpoint: "http://example.com",
		version:  dispatchFor(v),
	}
}

func NewMockResponse(statusCode int, r *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       ioutil.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    r,
	}
}
