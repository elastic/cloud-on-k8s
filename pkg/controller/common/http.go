// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"net/http"
)

const (
	InternalProductRequestHeaderString = "x-elastic-product-origin: cloud"
	internalProductRequestHeaderKey    = "x-elastic-product-origin"
	internalProductRequestHeaderValue  = "cloud"
)

// SetInternalProductRequestHeader sets header that specifies which internal product is making the request.
func SetInternalProductRequestHeader(req *http.Request) {
	if req == nil || req.Header == nil {
		return
	}

	req.Header.Set(internalProductRequestHeaderKey, internalProductRequestHeaderValue)
}
