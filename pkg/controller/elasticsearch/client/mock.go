// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
)

type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func NewMockClient(v version.Version, fn RoundTripFunc) Client {
	return NewMockClientWithUser(v, BasicAuth{}, fn)
}

func NewMockClientWithUser(v version.Version, u BasicAuth, fn RoundTripFunc) Client {
	baseClient := &baseClient{
		HTTP: &http.Client{
			Transport: fn,
		},
		Endpoint: "http://example.com",
		User:     u,
	}
	return versioned(baseClient, v)
}

func NewMockResponse(statusCode int, r *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       ioutil.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    r,
	}
}
