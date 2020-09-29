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
func LogReconciliationRun(log logr.Logger, request reconcile.Request, iteration *uint64) func() {
	currentIteration := atomic.AddUint64(iteration, 1)
	startTime := time.Now()

	log.Info("Starting reconciliation run", "iteration", currentIteration)

	return func() {
		totalTime := time.Since(startTime)
		log.Info("Ending reconciliation run", "iteration", currentIteration, "took", totalTime)
	}
}
