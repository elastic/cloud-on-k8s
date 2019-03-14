// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

import (
	"reflect"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test_nextTakesPrecedence(t *testing.T) {
	type args struct {
		current reconcile.Result
		next    reconcile.Result
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "identity",
			args: args{},
			want: false,
		},
		{
			name: "generic requeue takes precedence over no requeue",
			args: args{
				current: reconcile.Result{},
				next:    reconcile.Result{Requeue: true},
			},
			want: true,
		},
		{
			name: "shorter time to reconcile takes precedence",
			args: args{
				current: reconcile.Result{RequeueAfter: 1 * time.Hour},
				next:    reconcile.Result{RequeueAfter: 1 * time.Minute},
			},
			want: true,
		},
		{
			name: "specific requeue trumps generic requeue",
			args: args{
				current: reconcile.Result{Requeue: true},
				next:    reconcile.Result{RequeueAfter: 1 * time.Minute},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextResultTakesPrecedence(tt.args.current, tt.args.next); got != tt.want {
				t.Errorf("nextResultTakesPrecedence() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResults(t *testing.T) {
	tests := []struct {
		name string
		args []reconcile.Result
		want reconcile.Result
	}{
		{
			name: "none",
			args: nil,
			want: reconcile.Result{},
		},
		{
			name: "one",
			args: []reconcile.Result{{Requeue: true}},
			want: reconcile.Result{Requeue: true},
		},
		{
			name: "multiple",
			args: []reconcile.Result{{}, {Requeue: true}, {RequeueAfter: 1 * time.Second}},
			want: reconcile.Result{RequeueAfter: 1 * time.Second},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Results{
				results: tt.args,
			}
			if got, _ := r.Aggregate(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Aggregate() = %v, want %v", got, tt.want)
			}
		})
	}
}
