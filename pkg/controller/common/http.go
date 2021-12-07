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

// InternalProductHTTPHeaders is a slice that contains header that specifies which product is making the request.
var InternalProductHTTPHeaders = []corev1.HTTPHeader{
	{
		Name:  internalProductRequestHeaderKey,
		Value: internalProductRequestHeaderValue,
	},
}

// SetInternalProductRequestHeader sets header that specifies which internal product is making the request.
func SetInternalProductRequestHeader(req *http.Request) {
	if req == nil || req.Header == nil {
		return
	}

	req.Header.Set(internalProductRequestHeaderKey, internalProductRequestHeaderValue)
}
