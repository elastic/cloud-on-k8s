// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nsmatch"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	cachemock "github.com/elastic/cloud-on-k8s/v3/pkg/utils/test/mock"
)

const testOperatorNS = "elastic-system"

// fakeNamespacedReconciler records Reconcile and OnNamespaceOutOfScope calls.
type fakeNamespacedReconciler struct {
	result reconcile.Result
	err    error

	reconciled []reconcile.Request
	outOfScope []types.NamespacedName
}

func (f *fakeNamespacedReconciler) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	f.reconciled = append(f.reconciled, request)
	return f.result, f.err
}

func (f *fakeNamespacedReconciler) OnNamespaceOutOfScope(resource types.NamespacedName) {
	f.outOfScope = append(f.outOfScope, resource)
}

var _ NamespacedReconciler = (*fakeNamespacedReconciler)(nil)

// failingLicenseChecker fails every license check. Also used in cases where the license must not
// be consulted at all: if it were, Reconcile would return its error and the test would catch it.
type failingLicenseChecker struct{ license.Checker }

func (failingLicenseChecker) EnterpriseFeaturesEnabled(context.Context) (bool, error) {
	return false, errors.New("license check failed")
}

// testMatcher builds a NamespaceMatcher on the selector env=prod, backed by a mock cache:
// namespaces listed in matched resolve to labels satisfying the selector, all others resolve
// to no labels and therefore do not match.
func testMatcher(t *testing.T, matched ...string) *nsmatch.NamespaceMatcher {
	t.Helper()

	sel, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}})
	require.NoError(t, err)

	matchedSet := make(map[string]struct{}, len(matched))
	for _, ns := range matched {
		matchedSet[ns] = struct{}{}
	}

	mc := cachemock.NewCache(t)
	mc.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			key := args.Get(1).(client.ObjectKey) //nolint:forcetypeassert
			ns := args.Get(2).(*corev1.Namespace) //nolint:forcetypeassert
			ns.ObjectMeta = metav1.ObjectMeta{Name: key.Name}
			if _, ok := matchedSet[key.Name]; ok {
				ns.Labels = map[string]string{"env": "prod"}
			}
		}).
		Return(nil).
		Maybe()

	m := nsmatch.NewNamespaceMatcher(sel, testOperatorNS)
	m.SetCache(mc)
	return m
}

func Test_namespacedReconcilerWrapper_Reconcile(t *testing.T) {
	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns1", Name: "es1"}}
	innerResult := reconcile.Result{RequeueAfter: 42 * time.Second}

	tests := []struct {
		name           string
		matchedNS      []string
		request        reconcile.Request
		inner          *fakeNamespacedReconciler
		licenseChecker license.Checker

		wantResult     reconcile.Result
		wantErr        string
		wantReconciled []reconcile.Request
		wantOutOfScope []types.NamespacedName
		wantEvent      bool
	}{
		{
			name:      "namespace in scope with enterprise license: delegates to inner reconciler",
			matchedNS: []string{"ns1"},
			request:   request,
			inner:     &fakeNamespacedReconciler{result: innerResult},
			// license is checked and enabled
			licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
			wantResult:     innerResult,
			wantReconciled: []reconcile.Request{request},
		},
		{
			name:           "inner reconciler error is propagated",
			matchedNS:      []string{"ns1"},
			request:        request,
			inner:          &fakeNamespacedReconciler{err: errors.New("inner failed")},
			licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
			wantErr:        "inner failed",
			wantReconciled: []reconcile.Request{request},
		},
		{
			name:    "namespace out of scope: skips reconciliation, notifies inner, no license check",
			request: request, // ns1 not in matchedNS
			inner:   &fakeNamespacedReconciler{result: innerResult},
			// would fail the reconciliation if consulted
			licenseChecker: failingLicenseChecker{},
			wantResult:     reconcile.Result{},
			wantOutOfScope: []types.NamespacedName{request.NamespacedName},
		},
		{
			name:           "operator namespace is always in scope",
			request:        reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testOperatorNS, Name: "es1"}},
			inner:          &fakeNamespacedReconciler{result: innerResult},
			licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
			wantResult:     innerResult,
			wantReconciled: []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: testOperatorNS, Name: "es1"}}},
		},
		{
			name:           "license check error: reconciliation fails without calling inner",
			matchedNS:      []string{"ns1"},
			request:        request,
			inner:          &fakeNamespacedReconciler{result: innerResult},
			licenseChecker: failingLicenseChecker{},
			wantErr:        "license check failed",
		},
		{
			name:           "enterprise features disabled: requeues and emits event without calling inner",
			matchedNS:      []string{"ns1"},
			request:        request,
			inner:          &fakeNamespacedReconciler{result: innerResult},
			licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: false},
			wantResult:     reconcile.Result{RequeueAfter: 5 * time.Minute},
			wantEvent:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := toolsevents.NewFakeRecorder(10)
			r := &namespacedReconcilerWrapper{
				inner: tt.inner,
				parameters: operator.Parameters{
					OperatorNamespace: testOperatorNS,
					NamespaceMatcher:  testMatcher(t, tt.matchedNS...),
				},
				licenseChecker: tt.licenseChecker,
				recorder:       recorder,
			}

			result, err := r.Reconcile(t.Context(), tt.request)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantResult, result)
			require.Equal(t, tt.wantReconciled, tt.inner.reconciled)
			require.Equal(t, tt.wantOutOfScope, tt.inner.outOfScope)

			if tt.wantEvent {
				select {
				case e := <-recorder.Events:
					require.True(t, strings.Contains(e, license.EventInvalidLicense), "unexpected event: %s", e)
				default:
					t.Fatal("expected an InvalidLicense event, got none")
				}
			} else {
				require.Empty(t, recorder.Events)
			}
		})
	}
}

// a cache Get error must be treated as "namespace out of scope": skip and notify the inner reconciler.
func Test_namespacedReconcilerWrapper_Reconcile_cacheError(t *testing.T) {
	sel, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}})
	require.NoError(t, err)

	mc := cachemock.NewCache(t)
	mc.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("cache error"))

	matcher := nsmatch.NewNamespaceMatcher(sel, testOperatorNS)
	matcher.SetCache(mc)

	inner := &fakeNamespacedReconciler{}
	r := &namespacedReconcilerWrapper{
		inner:          inner,
		parameters:     operator.Parameters{OperatorNamespace: testOperatorNS, NamespaceMatcher: matcher},
		licenseChecker: failingLicenseChecker{},
		recorder:       toolsevents.NewFakeRecorder(10),
	}

	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns1", Name: "es1"}}
	result, err := r.Reconcile(t.Context(), request)

	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, result)
	require.Empty(t, inner.reconciled)
	require.Equal(t, []types.NamespacedName{request.NamespacedName}, inner.outOfScope)
}
