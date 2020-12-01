// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package tracing

import (
	"context"
	"runtime"
	"strings"

	"go.elastic.co/apm"
)

const (
	SpanTypeApp string = "app"
)

func Span(ctx context.Context) func() {
	pc, _, _, ok := runtime.Caller(1)

	name := "unknown_function"
	if ok {
		f := runtime.FuncForPC(pc)
		name := f.Name()
		// cut module and package name, leave only func name

		lastDot := strings.LastIndex(name, ".")
		// if something went wrong and dot is not present or last, let's not crash the operator and use full name instead
		if 0 <= lastDot && lastDot < len(name)-1 {
			name = name[lastDot+1:]
		}
	}

	span, _ := apm.StartSpan(ctx, name, SpanTypeApp)
	return func() {
		defer span.End()
	}
}
