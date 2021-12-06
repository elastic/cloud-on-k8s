// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"net/http"

	corev1 "k8s.io/api/core/v1"
)

const (
	internalProductRequestHeaderKey   = "x-elastic-product-origin"
	internalProductRequestHeaderValue = "cloud"
)

// SetInternalProductRequestHeader sets header that specifies which internal product is making the request.
func SetInternalProductRequestHeader(req *http.Request) {
	req.Header.Set(GetInternalProductRequestHeaderKey(), GetInternalProductRequestHeaderValue())
}

// CreateInternalProductHTTPHeaders creates header slice that contains header that specifies which internal
// product is making the request.
func CreateInternalProductHTTPHeaders() []corev1.HTTPHeader {
	return []corev1.HTTPHeader{
		{
			Name:  GetInternalProductRequestHeaderKey(),
			Value: GetInternalProductRequestHeaderValue(),
		},
	}
}

func GetInternalProductRequestHeaderKey() string {
	return internalProductRequestHeaderKey
}

func GetInternalProductRequestHeaderValue() string {
	return internalProductRequestHeaderValue
}
