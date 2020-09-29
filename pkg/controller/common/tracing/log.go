// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package tracing

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	errors "github.com/pkg/errors"
	"go.elastic.co/apm"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
)

// NewLogAdapter returns an implementation of the log interface expected by the APM agent.
func NewLogAdapter(log logr.Logger) apm.Logger {
	return &logAdapter{
		log: log,
	}
}

type logAdapter struct {
	log logr.Logger
}

func (l *logAdapter) Errorf(format string, args ...interface{}) {
	l.log.Error(errors.Errorf(format, args...), "")
}

func (l *logAdapter) Warningf(format string, args ...interface{}) {
	l.log.V(-1).Info(fmt.Sprintf(format, args...))
}

func (l *logAdapter) Debugf(format string, args ...interface{}) {
	l.log.V(1).Info(fmt.Sprintf(format, args...))
}

var (
	_ apm.Logger        = &logAdapter{}
	_ apm.WarningLogger = &logAdapter{}
)

// LoggerFromContext returns a logger from the context with tracing information added.
// TODO must init with SetLogger first?
func LoggerFromContext(ctx context.Context) logr.Logger {
	fields := TraceContextKV(ctx)
	return crlog.FromContext(ctx).WithValues(fields...)
}

// TraceContextKV returns logger key-values for the current trace context.
func TraceContextKV(ctx context.Context) []interface{} {
	tx := apm.TransactionFromContext(ctx)
	if tx == nil {
		return nil
	}

	traceCtx := tx.TraceContext()
	fields := []interface{}{"trace.id", traceCtx.Trace, "transaction.id", traceCtx.Span}

	if span := apm.SpanFromContext(ctx); span != nil {
		fields = append(fields, "span.id", span.TraceContext().Span)
	}

	return fields
}
