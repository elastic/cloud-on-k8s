// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmclientgo

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/pkg/errors"
	"go.elastic.co/apm/module/apmhttp/v2"
	"go.elastic.co/apm/v2"
)

// WrapRoundTripper returns a http.Roundtripper wrapping r, reporting each
// request as a span to Elastic APM, if the request's context contains a sampled transaction
// Allows an optional default transaction to be configured for requests where context cannot be controlled
// for example client-go's cache management
func WrapRoundTripper(r http.RoundTripper, o ...ClientOption) http.RoundTripper {
	if r == nil {
		r = http.DefaultTransport
	}
	rt := &roundTripper{r: r}
	// apply any client options
	for _, o := range o {
		o(rt)
	}
	return rt
}

type roundTripper struct {
	r           http.RoundTripper
	defaultTxFn func() *apm.Transaction
}

func (r roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	tx := apm.TransactionFromContext(ctx)
	if tx == nil && r.defaultTxFn != nil {
		tx = r.defaultTxFn()
		if tx != nil {
			defer tx.End()
		}
	}
	if tx == nil {
		return r.r.RoundTrip(req)
	}
	traceContext := tx.TraceContext()
	if !tx.Sampled() {
		apmhttp.SetHeaders(req, traceContext, false)
		return r.r.RoundTrip(req)
	}

	propagateLegacyHeader := tx.ShouldPropagateLegacyHeader()
	requestName := requestName(req)
	name := spanName(requestName)
	span := tx.StartSpan(name, "db.kubernetes", apm.SpanFromContext(ctx))

	if span.Dropped() {
		span.End()
		apmhttp.SetHeaders(req, traceContext, propagateLegacyHeader)
		return r.r.RoundTrip(req)
	}

	traceContext = span.TraceContext()
	ctx = apm.ContextWithSpan(ctx, span)
	req = apmhttp.RequestWithContext(ctx, req)
	span.Context.SetHTTPRequest(req)
	span.Context.SetDestinationService(apm.DestinationServiceSpanContext{
		Name:     "Kubernetes API server",
		Resource: "Kubernetes",
	})
	span.Context.SetDatabase(apm.DatabaseSpanContext{
		Statement: requestName,
		Type:      "kubernetes",
	})

	apmhttp.SetHeaders(req, traceContext, propagateLegacyHeader)
	resp, err := r.r.RoundTrip(req)
	if err != nil {
		span.End()
	} else {
		span.Context.SetHTTPStatusCode(resp.StatusCode)
		resp.Body = &responseBody{span: span, body: resp.Body}
	}
	return resp, err
}

// CloseIdleConnections calls r.r.CloseIdleConnections if the method exists.
func (r *roundTripper) CloseIdleConnections() {
	type closeIdler interface {
		CloseIdleConnections()
	}
	if tr, ok := r.r.(closeIdler); ok {
		tr.CloseIdleConnections()
	}
}

// CancelRequest calls r.r.CancelRequest(req) if the method exists.
func (r *roundTripper) CancelRequest(req *http.Request) {
	type cancelRequester interface {
		CancelRequest(*http.Request)
	}
	if r, ok := r.r.(cancelRequester); ok {
		r.CancelRequest(req)
	}
}

type responseBody struct {
	span *apm.Span
	body io.ReadCloser
}

// Close closes the response body, and ends the span if it hasn't already been ended.
func (b *responseBody) Close() error {
	b.endSpan()
	return b.body.Close()
}

// Read reads from the response body, and ends the span when io.EOF is returned if
// the span hasn't already been ended.
func (b *responseBody) Read(p []byte) (n int, err error) {
	n, err = b.body.Read(p)
	if errors.Is(err, io.EOF) {
		b.endSpan()
	}
	return n, err
}

func (b *responseBody) endSpan() {
	addr := (*unsafe.Pointer)(unsafe.Pointer(&b.span))
	if old := atomic.SwapPointer(addr, nil); old != nil {
		(*apm.Span)(old).End()
	}
}

func spanName(reqName string) string {
	const prefix = "Kubernetes:"
	var b strings.Builder
	b.Grow(len(prefix) + 1 + len(reqName))
	b.WriteString(prefix)
	b.WriteRune(' ')
	b.WriteString(reqName)
	return b.String()
}

func requestName(req *http.Request) string {
	statement := req.Method
	numSegments := 2
	// add a bit more context in the summary for PUT requests e.g. namespace
	if req.Method == "PUT" {
		numSegments = 3
	}

	pathSegments := strings.Split(req.URL.Path, "/")
	path := strings.Join(pathSegments[len(pathSegments)-numSegments:], "/")
	// let's call out watch requests explicitly
	if watch := req.URL.Query().Get("watch"); watch == "true" {
		statement = "WATCH"
	}
	var b strings.Builder
	b.Grow(len(statement) + 1 + len(path))
	b.WriteString(statement)
	b.WriteRune(' ')
	b.WriteString(path)
	return b.String()
}

// ClientOption sets options for tracing client requests.
type ClientOption func(*roundTripper)

// WithDefaultTransaction configures the roundtripper to start a new APM transaction if no transaction is currently running
// using the factory function f.
func WithDefaultTransaction(f func() *apm.Transaction) ClientOption {
	return ClientOption(func(rt *roundTripper) {
		rt.defaultTxFn = f
	})
}
