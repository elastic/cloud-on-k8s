// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package tracing

import (
	"context"

	"go.elastic.co/apm/v2"
)

// CaptureError wraps APM agent func of the same name and auto-sends, returning the original error.
func CaptureError(ctx context.Context, err error) error {
	if ctx != nil {
		if capturedErr := apm.CaptureError(ctx, err); capturedErr != nil {
			capturedErr.Send()
		}
	}
	return err // dropping the apm wrapper here
}
