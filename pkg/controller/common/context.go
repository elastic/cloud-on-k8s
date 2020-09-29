// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
)

// NewReconciliationContext creates a new context for a reconciliation function.
func NewReconciliationContext(logger logr.Logger, name types.NamespacedName, kind string) context.Context {
	newLogger := logger.WithValues("labels", map[string]string{"kind": kind, "resource": name.Name, "namespace": name.Namespace})
	return crlog.IntoContext(context.Background(), newLogger)
}

// NewMockContext creates a context to use for tests where a context with a logger is required
func NewMockContext() context.Context {
	return crlog.IntoContext(context.Background(), crlog.Log.WithName("mock"))
}
