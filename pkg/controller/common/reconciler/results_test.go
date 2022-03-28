// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reconciler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestResults(t *testing.T) {
	args := []struct {
		kind   resultKind
		result reconcile.Result
	}{
		{kind: noqueueKind, result: reconcile.Result{}},                                               // 0
		{kind: specificKind, result: reconcile.Result{RequeueAfter: 10 * time.Second}},                // 1
		{kind: specificKind, result: reconcile.Result{RequeueAfter: 20 * time.Second, Requeue: true}}, // 2
		{kind: genericKind, result: reconcile.Result{Requeue: true}},                                  // 3
	}

	wantRes := []struct {
		kind   resultKind
		result reconcile.Result
	}{
		{kind: noqueueKind, result: reconcile.Result{}},                                               // 0 & 0
		{kind: specificKind, result: reconcile.Result{RequeueAfter: 10 * time.Second}},                // 0 & 1
		{kind: specificKind, result: reconcile.Result{RequeueAfter: 20 * time.Second, Requeue: true}}, // 0 & 2
		{kind: genericKind, result: reconcile.Result{Requeue: true}},                                  // 0 & 3

		{kind: specificKind, result: reconcile.Result{RequeueAfter: 10 * time.Second}}, // 1 & 0
		{kind: specificKind, result: reconcile.Result{RequeueAfter: 10 * time.Second}}, // 1 & 1
		{kind: specificKind, result: reconcile.Result{RequeueAfter: 10 * time.Second}}, // 1 & 2
		{kind: genericKind, result: reconcile.Result{Requeue: true}},                   // 1 & 3

		{kind: specificKind, result: reconcile.Result{RequeueAfter: 20 * time.Second, Requeue: true}}, // 2 & 0
		{kind: specificKind, result: reconcile.Result{RequeueAfter: 10 * time.Second}},                // 2 & 1
		{kind: specificKind, result: reconcile.Result{RequeueAfter: 20 * time.Second, Requeue: true}}, // 2 & 2
		{kind: genericKind, result: reconcile.Result{Requeue: true}},                                  // 2 & 3

		{kind: genericKind, result: reconcile.Result{Requeue: true}}, // 3 & 0
		{kind: genericKind, result: reconcile.Result{Requeue: true}}, // 3 & 1
		{kind: genericKind, result: reconcile.Result{Requeue: true}}, // 3 & 2
		{kind: genericKind, result: reconcile.Result{Requeue: true}}, // 3 & 3
	}

	for i, arg := range args {
		t.Run(fmt.Sprintf("kindOf_%d", i), func(t *testing.T) {
			require.Equal(t, arg.kind, kindOf(arg.result))
		})
	}

	err1 := errors.New("err1")
	err2 := errors.New("err2")

	idx := 0
	for i, a := range args {
		for j, b := range args {
			// test mergeResult method
			t.Run(fmt.Sprintf("mergeResult_%d_%d", i, j), func(t *testing.T) {
				have := &Results{currKind: a.kind, currResult: ReconciliationState{Result: a.result}}
				have.mergeResult(b.kind, ReconciliationState{Result: b.result})
				want := wantRes[idx]
				require.Equal(t, want.kind, have.currKind, "Kinds do not match")
				require.Equal(t, want.result, have.currResult.Result, "Results do not match")
			})

			// test WithResults method
			t.Run(fmt.Sprintf("withResults_%d_%d", i, j), func(t *testing.T) {
				this := &Results{currKind: a.kind, currResult: ReconciliationState{Result: a.result}, errors: []error{err1}}
				that := &Results{currKind: b.kind, currResult: ReconciliationState{Result: b.result}, errors: []error{err2}}
				have := this.WithResults(that)
				want := wantRes[idx]

				require.Equal(t, want.kind, have.currKind, "Unexpected kind")
				require.Equal(t, want.result, have.currResult.Result, "Unexpected result")
				require.Equal(t, []error{err1, err2}, have.errors, "Errors not merged")
			})

			idx++
		}
	}
}

func TestResultsAggregate(t *testing.T) {
	testCases := []struct {
		name    string
		results *Results
		want    reconcile.Result
	}{
		{
			name:    "noqueue result",
			results: &Results{},
			want:    reconcile.Result{},
		},
		{
			name:    "generic result",
			results: &Results{currResult: ReconciliationState{Result: reconcile.Result{Requeue: true}}, currKind: genericKind},
			want:    reconcile.Result{Requeue: true},
		},
		{
			name:    "specific result",
			results: &Results{currResult: ReconciliationState{Result: reconcile.Result{RequeueAfter: 24 * time.Hour}}, currKind: specificKind},
			want:    reconcile.Result{RequeueAfter: 24 * time.Hour},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			have, _ := tc.results.Aggregate()
			require.Equal(t, tc.want, have)
		})
	}
}

func TestResultsHasError(t *testing.T) {
	r := &Results{
		ctx: context.Background(),
	}
	require.False(t, r.HasError())

	r = r.WithError(nil)
	require.False(t, r.HasError())

	r = r.WithError(errors.New("some error"))
	require.True(t, r.HasError())
}

func TestResults_IsReconciled(t *testing.T) {
	tests := []struct {
		name           string
		results        *Results
		wantReconciled bool
		wantReason     string
	}{
		{
			name: "Ignore RequeueAfter if WithReconciliationState is called",
			results: (&Results{}).
				WithReconciliationState(RequeueAfter(time.Duration(42)).ReconciliationComplete()),
			wantReconciled: true,
		},
		{
			name: "Do not ignore RequeueAfter if ReconciliationComplete has lower priority",
			results: (&Results{}).
				WithReconciliationState(RequeueAfter(time.Duration(84)).ReconciliationComplete()).
				WithReconciliationState(RequeueAfter(time.Duration(42))),
			wantReconciled: false,
		},
		{
			name: "Do not ignore RequeueAfter if ReconciliationComplete has higher priority",
			results: (&Results{}).
				WithReconciliationState(RequeueAfter(time.Duration(84)).ReconciliationComplete()).
				WithReconciliationState(RequeueAfter(time.Duration(96))),
			wantReconciled: false,
		},
		{
			name: "Error before",
			results: (&Results{}).
				WithError(errors.New("foo")).
				WithReconciliationState(RequeueAfter(time.Duration(84)).ReconciliationComplete()),
			wantReconciled: false,
			wantReason:     "foo",
		},
		{
			name: "Forced requeue",
			results: (&Results{}).
				WithResult(reconcile.Result{
					Requeue:      true,
					RequeueAfter: 0,
				}).
				WithReconciliationState(RequeueAfter(time.Duration(84)).ReconciliationComplete()),
			wantReconciled: false,
		},
		{
			name: "Error after",
			results: (&Results{}).
				WithReconciliationState(RequeueAfter(time.Duration(84)).ReconciliationComplete()).
				WithError(errors.New("foo2")),
			wantReconciled: false,
			wantReason:     "foo2",
		},
		{
			name: "WithReason called twice",
			results: (&Results{}).
				WithReconciliationState(RequeueAfter(time.Duration(42)).WithReason("a better reason")).
				WithReconciliationState(RequeueAfter(time.Duration(84)).WithReason("my reason")),
			wantReconciled: false,
			wantReason:     "a better reason",
		},
		{
			name: "Error has more priority than WithReason",
			results: (&Results{}).
				WithReconciliationState(RequeueAfter(time.Duration(42)).WithReason("a better reason")).
				WithError(errors.New("bar")).
				WithReconciliationState(RequeueAfter(time.Duration(84)).WithReason("my reason")),
			wantReconciled: false,
			wantReason:     "bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReconciled, gotReason := tt.results.IsReconciled()
			if gotReconciled != tt.wantReconciled {
				t.Errorf("Results.IsReconciled() got = %v, want %v", gotReconciled, tt.wantReconciled)
			}
			if gotReason != tt.wantReason {
				t.Errorf("Results.IsReconciled() got1 = %v, want %v", gotReason, tt.wantReason)
			}
		})
	}
}
