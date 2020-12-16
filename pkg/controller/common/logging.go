// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// LogReconciliationRun is the common logging function used to record a reconciliation run.
func LogReconciliationRun(log logr.Logger, request reconcile.Request, nameField string, iteration *uint64) func() {
	currentIteration := atomic.AddUint64(iteration, 1)
	startTime := time.Now()
	log.Info("Starting reconciliation run", "iteration", currentIteration, "namespace", request.Namespace, nameField, request.Name)
	return func() {
		totalTime := time.Since(startTime)
		log.Info("Ending reconciliation run", "iteration", currentIteration, "namespace", request.Namespace, nameField, request.Name, "took", totalTime)
	}
}

// LogReconciliationRunNoSideEffects is the common logging function used to record a reconciliation run, it doesn't
// increment the iteration. When all controllers move away from package level loggers and move to using one from the
// context, the other logging function (LogReconciliationRun) can be removed in favor of this one.
func LogReconciliationRunNoSideEffects(log logr.Logger) func() {
	startTime := time.Now()
	log.Info("Starting reconciliation run")
	return func() {
		totalTime := time.Since(startTime)
		log.Info("Ending reconciliation run", "took", totalTime)
	}
}
