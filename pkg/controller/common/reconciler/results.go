// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

import (
	"time"

	k8serrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MaximumRequeueAfter is the maximum period of time in which we requeue a reconciliation.
const MaximumRequeueAfter = 10 * time.Hour

// Results collects intermediate results of a reconciliation run and any errors that occurred.
type Results struct {
	results []reconcile.Result
	errors  []error
}

// HasError returns true if Results contains one or more errors.
func (r *Results) HasError() bool {
	return len(r.errors) > 0
}

// WithResults appends the results and error from the other Results.
func (r *Results) WithResults(other *Results) *Results {
	if other != nil {
		r.results = append(r.results, other.results...)
		r.errors = append(r.errors, other.errors...)
	}
	return r
}

// WithError adds an error to the results.
func (r *Results) WithError(err error) *Results {
	if err != nil {
		r.errors = append(r.errors, err)
	}
	return r
}

// WithResult adds a result to the results.
func (r *Results) WithResult(res reconcile.Result) *Results {
	r.results = append(r.results, res)
	return r
}

// Apply applies the output of a reconciliation step to the results. The step outcome is implicitly considered
// recoverable as we just record the results and continue.
func (r *Results) Apply(step string, recoverableStep func() (reconcile.Result, error)) *Results {
	result, err := recoverableStep()
	if err != nil {
		log.Info("Recoverable error during step, continuing", "step", step, "error", err)
	}
	return r.WithError(err).WithResult(result)
}

// Aggregate compares the collected results with each other and returns the most specific one.
// Where specific means requeue at a given time is more specific then generic requeue which is more specific
// than no requeue. It also returns any errors recorded.
// The aggregated `result.RequeueAfter` period will not be larger than MaximumRequeueAfter.
func (r *Results) Aggregate() (reconcile.Result, error) {
	var current reconcile.Result
	for _, next := range r.results {
		if nextResultTakesPrecedence(current, next) {
			current = next
		}
	}
	if current.RequeueAfter > MaximumRequeueAfter {
		// A client-go leaky timer issue will cause memory leaks for long requeue periods,
		// see https://github.com/elastic/cloud-on-k8s/issues/1984.
		// To prevent this from happening, let's restrict the requeue to a fixed short-term value.
		// TODO: remove once https://github.com/kubernetes/client-go/issues/701 is fixed.
		current.RequeueAfter = MaximumRequeueAfter
	}
	return current, k8serrors.NewAggregate(r.errors)
}

// nextResultTakesPrecedence compares the current reconciliation result with the proposed one,
// and returns true if the current result should be replaced by the proposed one.
func nextResultTakesPrecedence(current, next reconcile.Result) bool {
	if current == next {
		return false // no need to replace the result
	}
	if next.Requeue && !current.Requeue && current.RequeueAfter <= 0 {
		return true // next requests requeue current does not, next takes precedence
	}
	if next.RequeueAfter > 0 && (current.RequeueAfter == 0 || next.RequeueAfter < current.RequeueAfter) {
		return true // next requests a requeue and current does not or wants it only later
	}
	return false // default case
}
