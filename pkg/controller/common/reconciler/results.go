// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reconciler

import (
	"context"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
)

type resultKind int

const (
	noqueueKind  resultKind = iota // reconcile.Result{}
	specificKind                   // reconcile.Result{RequeueAfter: x}
	genericKind                    // reconcile.Result{Requeue: true}
)

func kindOf(r reconcile.Result) resultKind {
	switch {
	case r.RequeueAfter > 0:
		return specificKind
	case r.Requeue:
		return genericKind
	default:
		return noqueueKind
	}
}

// Results collects intermediate results of a reconciliation run and any errors that occurred.
type Results struct {
	currResult ReconciliationState
	currKind   resultKind
	errors     []error
	ctx        context.Context
}

var Requeue = ReconciliationState{Result: reconcile.Result{Requeue: true}}

func RequeueAfter(requeueAfter time.Duration) ReconciliationState {
	return ReconciliationState{
		incomplete: true,
		Result: reconcile.Result{
			RequeueAfter: requeueAfter,
		},
	}
}

// ReconciliationState extends a reconciliation result with an optional reason that can be surfaced in the status.
type ReconciliationState struct {
	reconcile.Result

	// incomplete can be used to mark the current reconciliation as complete even if RequeueAfter is set.
	incomplete bool

	// reason is a string that might be surfaced to the user in the resource status.
	reason string
}

func (r ReconciliationState) WithReason(reason string) ReconciliationState {
	return ReconciliationState{
		Result:     r.Result,
		reason:     reason,
		incomplete: true,
	}
}

func (r ReconciliationState) ReconciliationComplete() ReconciliationState {
	return ReconciliationState{
		Result:     r.Result,
		reason:     r.reason,
		incomplete: false,
	}
}

func NewResult(ctx context.Context) *Results {
	return &Results{
		ctx: ctx,
	}
}

// HasError returns true if Results contains one or more errors.
func (r *Results) HasError() bool {
	if r == nil {
		return false
	}
	return len(r.errors) > 0
}

// WithResults appends the results and error from the other Results.
func (r *Results) WithResults(other *Results) *Results {
	if other != nil {
		r.mergeResult(other.currKind, other.currResult)
		r.errors = append(r.errors, other.errors...)
	}
	return r
}

// WithError adds an error to the results.
func (r *Results) WithError(err error) *Results {
	if err != nil {
		r.errors = append(r.errors, tracing.CaptureError(r.ctx, err))
	}
	return r
}

// WithResult adds a result to the results.
func (r *Results) WithResult(res reconcile.Result) *Results {
	incomplete := res.Requeue || !res.IsZero()
	r.WithReconciliationState(ReconciliationState{incomplete: incomplete, Result: res})
	return r
}

// WithReconciliationState adds a result and related state information to the results.
func (r *Results) WithReconciliationState(res ReconciliationState) *Results {
	kind := kindOf(res.Result)
	r.mergeResult(kind, res)
	return r
}

// mergeResult updates the current result if the other result has higher priority.
// Order of priority is: noqueue < specific < generic
// When there are two specific results, the one with the lowest RequeueAfter takes precedence.
func (r *Results) mergeResult(kind resultKind, res ReconciliationState) {
	switch {
	case kind > r.currKind:
		r.currKind = kind
		r.currResult = res
	case kind == specificKind && r.currKind == specificKind:
		if res.Result.RequeueAfter < r.currResult.Result.RequeueAfter {
			r.currResult = res
		}
	}
	// Reconciliation is considered as incomplete as soon as it has been reported, whatever the priority of the result.
	r.currResult.incomplete = r.currResult.incomplete || res.incomplete
}

// Aggregate returns the highest priority reconcile result and any errors seen so far.
func (r *Results) Aggregate() (reconcile.Result, error) {
	return r.currResult.Result, k8serrors.NewAggregate(r.errors)
}

// IsReconciled returns true if no error has been reported and if RequeueAfter is 0.
// It also returns true if ReconciliationComplete has been called while setting RequeueAfter to something
// greater than 0, in which case Requeue and RequeueAfter are ignored.
func (r *Results) IsReconciled() (bool, string) {
	if r.HasError() {
		err := k8serrors.NewAggregate(r.errors)
		return false, err.Error()
	}
	if !r.currResult.incomplete {
		return true, ""
	}
	return !(r.currResult.Result.Requeue || r.currResult.Result.RequeueAfter > 0), r.currResult.reason
}
