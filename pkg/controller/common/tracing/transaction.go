// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package tracing

import (
	"context"

	"go.elastic.co/apm/v2"
)

type TxType string

const (
	ReconciliationTxType TxType = "reconciliation"
	PeriodicTxType       TxType = "periodic"
	RunOnceTxType        TxType = "run-once"
)

// NewContextTransaction starts a new transaction and sets up a new context with that transaction that also contains the related
// APM agent's tracer.
func NewContextTransaction(ctx context.Context, t *apm.Tracer, txType TxType, txName string, labels map[string]string) context.Context {
	if t == nil {
		return ctx // apm turned off
	}

	tx := t.StartTransaction(txName, string(txType))
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
