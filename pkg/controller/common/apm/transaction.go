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

func NewTransaction(t *apm.Tracer, name types.NamespacedName, txType string) (*apm.Transaction, context.Context) {
	if t == nil {
		return nil, context.Background() // apm turned off
	}
	tx := t.StartTransaction(name.String(), txType)
	ctx := apm.ContextWithTransaction(context.Background(), tx)
	// also add the tracer as we need to start async transactions deep inside the call hierarchy
	// DISCUSS: alternative would be to explicitly pass the tracer down the call chain as an explicit arg
	return tx, context.WithValue(ctx, tracer{}, t)
}

func EndTransaction(tx *apm.Transaction) {
	if tx != nil {
		tx.End()
	}
}

func TracerFromContext(ctx context.Context) *apm.Tracer {
	tracer, _ := ctx.Value(tracer{}).(*apm.Tracer)
	return tracer
}
