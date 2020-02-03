// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

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
				have := &Results{currKind: a.kind, currResult: a.result}
				have.mergeResult(b.kind, b.result)
				want := wantRes[idx]
				require.Equal(t, want.kind, have.currKind, "Kinds do not match")
				require.Equal(t, want.result, have.currResult, "Results do not match")
			})

			// test WithResults method
			t.Run(fmt.Sprintf("withResults_%d_%d", i, j), func(t *testing.T) {
				this := &Results{currKind: a.kind, currResult: a.result, errors: []error{err1}}
				that := &Results{currKind: b.kind, currResult: b.result, errors: []error{err2}}
				have := this.WithResults(that)
				want := wantRes[idx]

				require.Equal(t, want.kind, have.currKind, "Unexpected kind")
				require.Equal(t, want.result, have.currResult, "Unexpected result")
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
			results: &Results{currResult: reconcile.Result{Requeue: true}, currKind: genericKind},
			want:    reconcile.Result{Requeue: true},
		},
		{
			name:    "specific result under MaximumRequeueAfter",
			results: &Results{currResult: reconcile.Result{RequeueAfter: 1 * time.Hour}, currKind: specificKind},
			want:    reconcile.Result{RequeueAfter: 1 * time.Hour},
		},
		{
			name:    "specific result over MaximumRequeueAfter",
			results: &Results{currResult: reconcile.Result{RequeueAfter: 24 * time.Hour}, currKind: specificKind},
			want:    reconcile.Result{RequeueAfter: MaximumRequeueAfter},
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
