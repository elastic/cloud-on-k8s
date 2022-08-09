// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package tracing

import (
	"net/http"
	"strings"
)

// RequestName returns method and path to be used in APM span names.
func RequestName(request *http.Request) string {
	if request == nil {
		return ""
	}
	var b strings.Builder
	b.Grow(len(request.Method) + len(request.URL.Path) + 1)
	b.WriteString(request.Method)
	b.WriteRune(' ')
	b.WriteString(request.URL.Path)
	return b.String()
}
