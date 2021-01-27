// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"context"
	"strconv"
	"sync/atomic"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	logconf "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"go.elastic.co/apm"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
		controllerName,
		request.String(),
		map[string]string{"iteration": itString})
	return logconf.InitInContext(
		newCtx,
		controllerName,
		"iteration", itString,
		"namespace", request.Namespace,
		nameField, request.Name)
}
