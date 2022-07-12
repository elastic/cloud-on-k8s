// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"time"

	"github.com/go-logr/logr"
)

// LogReconciliationRun is the common logging function used to record a reconciliation run, it doesn't
// increment the iteration.
func LogReconciliationRun(log logr.Logger) func() {
	startTime := time.Now()
	log.Info("Starting reconciliation run")
	return func() {
		totalTime := time.Since(startTime)
		log.Info("Ending reconciliation run", "took", totalTime)
	}
}
