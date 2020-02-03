// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

import (
	"context"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"go.elastic.co/apm"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MaximumRequeueAfter is the maximum period of time in which we requeue a reconciliation.
const MaximumRequeueAfter = 10 * time.Hour

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
	currResult reconcile.Result
	currKind   resultKind
	errors     []error
	ctx        context.Context
}

func NewResult(ctx context.Context) *Results {
	return &Results{
		ctx: ctx,
	}
}

// HasError returns true if Results contains one or more errors.
func (r *Results) HasError() bool {
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
	kind := kindOf(res)
	r.mergeResult(kind, res)
	return r
}

// mergeResult updates the current result if the other result has higher priority.
// Order of priority is: noqueue < specific < generic
// When there are two specific results, the one with the lowest RequeueAfter takes precedence.
func (r *Results) mergeResult(kind resultKind, res reconcile.Result) {
	switch {
	case kind > r.currKind:
		r.currKind = kind
		r.currResult = res
	case kind == r.currKind && kind == specificKind:
		if res.RequeueAfter < r.currResult.RequeueAfter {
			r.currResult = res
		}
	}
}

// Apply applies the output of a reconciliation step to the results. The step outcome is implicitly considered
// recoverable as we just record the results and continue.
func (r *Results) Apply(step string, recoverableStep func(context.Context) (reconcile.Result, error)) *Results {
	span, ctx := apm.StartSpan(r.ctx, step, tracing.SpanTypeApp)
	defer span.End()

	result, err := recoverableStep(ctx)
	if err != nil {
		log.Info("Recoverable error during step, continuing", "step", step, "error", err)
	}
	return r.WithError(err).WithResult(result)
}

// Aggregate returns the highest priority reconcile result and any errors seen so far.
func (r *Results) Aggregate() (reconcile.Result, error) {
	if r.currResult.RequeueAfter > MaximumRequeueAfter {
		// A client-go leaky timer issue will cause memory leaks for long requeue periods,
		// see https://github.com/elastic/cloud-on-k8s/issues/1984.
		// To prevent this from happening, let's restrict the requeue to a fixed short-term value.
		// TODO: remove once https://github.com/kubernetes/client-go/issues/701 is fixed.
		r.currResult.RequeueAfter = MaximumRequeueAfter
	}
	return r.currResult, k8serrors.NewAggregate(r.errors)
}
