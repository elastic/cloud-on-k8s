// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package namespace

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nsmatch"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func makeNamespace(name string, lbls map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: lbls},
	}
}

func nsRequest(name string) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Name: name}}
}

func TestReconciler_Reconcile(t *testing.T) {
	matchLabels := labels.Set{"managed": "true"}
	sel := labels.SelectorFromSet(matchLabels)

	matchingNS := makeNamespace("matching-ns", map[string]string{"managed": "true"})
	nonMatchingNS := makeNamespace("non-matching-ns", map[string]string{"managed": "false"})

	tests := []struct {
		name          string
		buildClient   func() k8s.Client
		preloadState  func(*nsmatch.NamespaceStates)
		request       reconcile.Request
		wantResult    reconcile.Result
		wantErr       bool
		wantBroadcast bool
		wantForgotten bool // whether the ns name should be absent from states after reconcile
	}{
		{
			name:        "namespace not found: state forgotten, no broadcast",
			buildClient: func() k8s.Client { return k8s.NewFakeClient() },
			preloadState: func(s *nsmatch.NamespaceStates) {
				s.Swap("deleted-ns", true) // pretend we knew about it
			},
			request:       nsRequest("deleted-ns"),
			wantResult:    reconcile.Result{},
			wantBroadcast: false,
			wantForgotten: true,
		},
		{
			name: "client error: error is propagated, no broadcast",
			buildClient: func() k8s.Client {
				return k8s.NewFakeClientBuilder().
					WithInterceptorFuncs(interceptor.Funcs{
						Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
							return errors.New("connection refused")
						},
					}).
					Build()
			},
			request:       nsRequest("any-ns"),
			wantErr:       true,
			wantBroadcast: false,
		},
		{
			name:          "first reconcile, namespace matches: broadcast (unknown -> matching)",
			buildClient:   func() k8s.Client { return k8s.NewFakeClient(matchingNS) },
			request:       nsRequest(matchingNS.Name),
			wantResult:    reconcile.Result{},
			wantBroadcast: true,
		},
		{
			name:          "first reconcile, namespace does not match: broadcast (unknown -> non-matching)",
			buildClient:   func() k8s.Client { return k8s.NewFakeClient(nonMatchingNS) },
			request:       nsRequest(nonMatchingNS.Name),
			wantResult:    reconcile.Result{},
			wantBroadcast: true,
		},
		{
			name:        "state unchanged: namespace still matches, no broadcast",
			buildClient: func() k8s.Client { return k8s.NewFakeClient(matchingNS) },
			preloadState: func(s *nsmatch.NamespaceStates) {
				s.Swap(matchingNS.Name, true)
			},
			request:       nsRequest(matchingNS.Name),
			wantResult:    reconcile.Result{},
			wantBroadcast: false,
		},
		{
			name:        "state unchanged: namespace still does not match, no broadcast",
			buildClient: func() k8s.Client { return k8s.NewFakeClient(nonMatchingNS) },
			preloadState: func(s *nsmatch.NamespaceStates) {
				s.Swap(nonMatchingNS.Name, false)
			},
			request:       nsRequest(nonMatchingNS.Name),
			wantResult:    reconcile.Result{},
			wantBroadcast: false,
		},
		{
			name:        "state change: namespace transitions matching -> non-matching, broadcast",
			buildClient: func() k8s.Client { return k8s.NewFakeClient(nonMatchingNS) },
			preloadState: func(s *nsmatch.NamespaceStates) {
				s.Swap(nonMatchingNS.Name, true) // previously matched
			},
			request:       nsRequest(nonMatchingNS.Name),
			wantResult:    reconcile.Result{},
			wantBroadcast: true,
		},
		{
			name:        "state change: namespace transitions non-matching -> matching, broadcast",
			buildClient: func() k8s.Client { return k8s.NewFakeClient(matchingNS) },
			preloadState: func(s *nsmatch.NamespaceStates) {
				s.Swap(matchingNS.Name, false) // previously did not match
			},
			request:       nsRequest(matchingNS.Name),
			wantResult:    reconcile.Result{},
			wantBroadcast: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states := nsmatch.NewNamespaceStates()
			if tt.preloadState != nil {
				tt.preloadState(states)
			}

			r := &reconciler{
				client:          tt.buildClient(),
				nsMatchNotifier: nsmatch.NewMatchNotifier(nil, sel, ""),
				states:          states,
			}
			sub := r.nsMatchNotifier.Subscribe()
			result, err := r.Reconcile(t.Context(), tt.request)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantResult, result)

			if tt.wantBroadcast {
				require.Len(t, sub, 1, "expected exactly one broadcast event")
				evt := <-sub
				assert.Equal(t, tt.request.Name, evt.Object.Name)
			} else {
				assert.Empty(t, sub, "expected no broadcast event")
			}

			if tt.wantForgotten {
				_, known := states.Swap(tt.request.Name, false)
				assert.False(t, known, "expected namespace state to be forgotten after reconcile")
			}
		})
	}
}
