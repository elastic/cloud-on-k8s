// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
)

type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func NewMockClient(v version.Version, fn RoundTripFunc) Interface {
	return NewMockClientWithUser(v, UserAuth{}, fn)
}

func NewMockClientWithUser(v version.Version, u UserAuth, fn RoundTripFunc) Interface {
	baseClient := &clientV6{
		HTTP: &http.Client{
			Transport: RoundTripFunc(fn),
		},
		Endpoint: "http://example.com",
		version:  v,
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
