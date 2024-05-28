// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"io"
	"net/http"
	"strings"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
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
		URLProvider: NewStaticURLProvider("http://example.com"),
		User:        u,
	}
	return versioned(baseClient, v)
}

func NewMockResponse(statusCode int, r *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    r,
	}
}
