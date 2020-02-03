// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package tracing

import (
	"context"

	"go.elastic.co/apm"
)

// CaptureError wraps APM agent func of the same name and auto-sends, returning the original error.
func CaptureError(ctx context.Context, err error) error {
	if ctx != nil {
		apm.CaptureError(ctx, err).Send()
	}
	return err // dropping the apm wrapper here
}
