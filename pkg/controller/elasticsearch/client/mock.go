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

func NewMockClient(v version.Version, fn RoundTripFunc) (Client, error) {
	return NewMockClientWithUser(v, UserAuth{}, fn)
}

func NewMockClientWithUser(v version.Version, u UserAuth, fn http.RoundTripper) (Client, error) {
	endpoint := "http://example.com"

	es, err := newElasticsearchClients(v, endpoint, u, fn)
	if err != nil {
		return nil, err
	}

	baseClient := &defaultClient{
		esVersion:            v,
		endpoint:             endpoint,
		userAuth:             u,
		elasticsearchClients: es,
	}
	return baseClient, nil
}

func NewMockResponse(statusCode int, r *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       ioutil.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    r,
	}
}
