// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package tracing

import (
	"context"

	"go.elastic.co/apm"
	"k8s.io/apimachinery/pkg/types"
)

// NewTransaction starts a new transaction and sets up a new context with that transaction that also contains the related
// APM agent's tracer.
func NewTransaction(ctx context.Context, t *apm.Tracer, name types.NamespacedName, txType string) (*apm.Transaction, context.Context) {
	if t == nil {
		return nil, ctx // apm turned off
	}
	tx := t.StartTransaction(name.String(), txType)
	return tx, apm.ContextWithTransaction(ctx, tx)
}

// EndTransaction nil safe version of APM agents tx.End()
func EndTransaction(tx *apm.Transaction) {
	if tx != nil {
		tx.End()
	}
}

// NewContextTransaction starts a new transaction and sets up a new context with that transaction that also contains the related
// APM agent's tracer.
func NewContextTransaction(ctx context.Context, t *apm.Tracer, txType, txName string, labels map[string]string) context.Context {
	if t == nil {
		return ctx // apm turned off
	}

	tx := t.StartTransaction(txName, txType)
	for k, v := range labels {
		tx.Context.SetLabel(k, v)
	}

	return apm.ContextWithTransaction(ctx, tx)
}

// EndContextTransaction nil safe version of APM agents tx.End()
func EndContextTransaction(ctx context.Context) {
	tx := apm.TransactionFromContext(ctx)
	if tx != nil {
		tx.End()
	}
}
