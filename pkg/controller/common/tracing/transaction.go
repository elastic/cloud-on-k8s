// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package tracing

import (
	"context"

	"go.elastic.co/apm"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewTransaction starts a new transaction and sets up a new context with that transaction that also contains the related
// APM agent's tracer.
func NewTransaction(t *apm.Tracer, name types.NamespacedName, txType string) (*apm.Transaction, context.Context) {
	if t == nil {
		return nil, context.Background() // apm turned off
	}
	tx := t.StartTransaction(name.String(), txType)
	ctx := apm.ContextWithTransaction(context.Background(), tx)
	return tx, ctx
}

// EndTransaction nil safe version of APM agents tx.End()
func EndTransaction(tx *apm.Transaction) {
	if tx != nil {
		tx.End()
	}
}

// ReconcilliationFn describes a reconciliation function.
type ReconcilliationFn func(context.Context, reconcile.Request) (reconcile.Result, error)

// TraceReconciliation instruments a reconciliation function for tracing
func TraceReconciliation(ctx context.Context, request reconcile.Request, kind string, fn ReconcilliationFn) (reconcile.Result, error) {
	t := Tracer()
	if t == nil {
		return fn(ctx, request)
	}

	n := request.NamespacedName.String()

	tx := t.StartTransaction(n, kind)
	defer tx.End()

	newCtx := apm.ContextWithTransaction(ctx, tx)
	result, err := fn(newCtx, request)

	return result, apm.CaptureError(newCtx, err)
}

// DoInSpan wraps the given function within a tracing span.
func DoInSpan(ctx context.Context, name string, fn func(context.Context) error) error {
	span, ctx := apm.StartSpan(ctx, name, SpanTypeApp)
	defer span.End()

	return apm.CaptureError(ctx, fn(ctx))
}
