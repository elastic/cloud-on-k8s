// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

import (
	"context"

	"go.elastic.co/apm"
	"k8s.io/apimachinery/pkg/types"
)

type tracer struct{}

// NewTransaction starts a new transaction and sets up a new context with that transaction that also contains the related
// APM agent's tracer.
func NewTransaction(t *apm.Tracer, name types.NamespacedName, txType string) (*apm.Transaction, context.Context) {
	if t == nil {
		return nil, context.Background() // apm turned off
	}
	tx := t.StartTransaction(name.String(), txType)
	ctx := apm.ContextWithTransaction(context.Background(), tx)
	// also add the tracer as we need to start new transactions deep inside the call hierarchy e.g. for Elasticsearch observers
	return tx, context.WithValue(ctx, tracer{}, t)
}

// EndTransaction nil safe version of APM agents tx.End()
func EndTransaction(tx *apm.Transaction) {
	if tx != nil {
		tx.End()
	}
}

// TracerFromContext retrieves an apm.Tracer from the context or nil.
func TracerFromContext(ctx context.Context) *apm.Tracer {
	tracer, _ := ctx.Value(tracer{}).(*apm.Tracer)
	return tracer
}
