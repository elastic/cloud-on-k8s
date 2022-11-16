// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"strconv"
	"sync/atomic"

	"go.elastic.co/apm/module/apmzap/v2"
	"go.elastic.co/apm/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	logconf "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// NewController creates a new controller with the given name, reconciler and parameters and registers it with the manager.
func NewController(mgr manager.Manager, name string, r reconcile.Reconciler, p operator.Parameters) (controller.Controller, error) {
	return controller.New(name, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: p.MaxConcurrentReconciles})
}

// NewReconciliationContext increments iteration, creates an apm transaction and initiates the logger. Returns context
// with apm transaction metadata and configured logger.
func NewReconciliationContext(
	ctx context.Context,
	iteration *uint64,
	tracer *apm.Tracer,
	controllerName, nameField string,
	request reconcile.Request,
) context.Context {
	it := atomic.AddUint64(iteration, 1)
	itString := strconv.FormatUint(it, 10)
	newCtx := tracing.NewContextTransaction(
		ctx,
		tracer,
		tracing.ReconciliationTxType,
		controllerName,
		map[string]string{"iteration": itString, "name": request.Name, "namespace": request.Namespace})

	// operator specific fields
	logFields := []interface{}{
		"iteration", itString,
		"namespace", request.Namespace,
		nameField, request.Name,
	}

	// tracing releated fields for log correlation
	for _, field := range apmzap.TraceContext(newCtx) {
		logFields = append(logFields, field.Key, field.Interface)
	}

	return logconf.InitInContext(
		newCtx,
		controllerName,
		logFields...,
	)
}
