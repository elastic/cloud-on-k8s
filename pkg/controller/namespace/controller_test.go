// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package namespace

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nsmatch"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/test/mock"
)

// errLicenseChecker embeds MockLicenseChecker and overrides EnterpriseFeaturesEnabled
// to return a configurable error.
type errLicenseChecker struct {
	license.MockLicenseChecker
	err error
}

func (e errLicenseChecker) EnterpriseFeaturesEnabled(context.Context) (bool, error) {
	return false, e.err
}

func makeNamespace(name string, lbls map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: lbls},
	}
}

func nsRequest(name string) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Name: name}}
}

func TestReconciler_doReconcile(t *testing.T) {
	sel := labels.SelectorFromSet(labels.Set{"managed": "true"})

	matchingNS := makeNamespace("matching-ns", map[string]string{"managed": "true"})
	nonMatchingNS := makeNamespace("non-matching-ns", map[string]string{"managed": "false"})

	tests := []struct {
		name          string
		buildCache    func(t *testing.T) cache.Cache
		preloadState  func(*nsmatch.NamespaceMatcher)
		request       reconcile.Request
		wantResult    reconcile.Result
		wantErr       bool
		wantBroadcast bool
	}{
		{
			name: "namespace not found: state forgotten, no broadcast",
			buildCache: func(t *testing.T) cache.Cache {
				mc := mock.NewCache(t)
				mc.OnGetSetNamespace(nil).Return(apierrors.NewNotFound(corev1.Resource("namespaces"), "deleted-ns"))
				return mc
			},
			preloadState: func(m *nsmatch.NamespaceMatcher) {
				m.Swap("deleted-ns", true)
			},
			request:       nsRequest("deleted-ns"),
			wantResult:    reconcile.Result{},
			wantBroadcast: false,
		},
		{
			name: "client error: error is propagated, no broadcast",
			buildCache: func(t *testing.T) cache.Cache {
				mc := mock.NewCache(t)
				mc.OnGetSetNamespace(nil).Return(errors.New("connection refused"))
				return mc
			},
			request:       nsRequest("any-ns"),
			wantErr:       true,
			wantBroadcast: false,
		},
		{
			name: "first reconcile, namespace matches: broadcast (unknown -> matching)",
			buildCache: func(t *testing.T) cache.Cache {
				mc := mock.NewCache(t)
				mc.OnGetSetNamespace(matchingNS.Labels).Return(nil)
				return mc
			},
			request:       nsRequest(matchingNS.Name),
			wantResult:    reconcile.Result{},
			wantBroadcast: true,
		},
		{
			name: "first reconcile, namespace does not match: no broadcast (unknown -> non-matching)",
			buildCache: func(t *testing.T) cache.Cache {
				mc := mock.NewCache(t)
				mc.OnGetSetNamespace(nonMatchingNS.Labels).Return(nil)
				return mc
			},
			request:       nsRequest(nonMatchingNS.Name),
			wantResult:    reconcile.Result{},
			wantBroadcast: false,
		},
		{
			name: "state unchanged: namespace still matches, no broadcast",
			buildCache: func(t *testing.T) cache.Cache {
				mc := mock.NewCache(t)
				mc.OnGetSetNamespace(matchingNS.Labels).Return(nil)
				return mc
			},
			preloadState: func(m *nsmatch.NamespaceMatcher) {
				m.Swap(matchingNS.Name, true)
			},
			request:       nsRequest(matchingNS.Name),
			wantResult:    reconcile.Result{},
			wantBroadcast: false,
		},
		{
			name: "state unchanged: namespace still does not match, no broadcast",
			buildCache: func(t *testing.T) cache.Cache {
				mc := mock.NewCache(t)
				mc.OnGetSetNamespace(nonMatchingNS.Labels).Return(nil)
				return mc
			},
			preloadState: func(m *nsmatch.NamespaceMatcher) {
				m.Swap(nonMatchingNS.Name, false)
			},
			request:       nsRequest(nonMatchingNS.Name),
			wantResult:    reconcile.Result{},
			wantBroadcast: false,
		},
		{
			name: "state change: namespace transitions matching -> non-matching, broadcast",
			buildCache: func(t *testing.T) cache.Cache {
				mc := mock.NewCache(t)
				mc.OnGetSetNamespace(nonMatchingNS.Labels).Return(nil)
				return mc
			},
			preloadState: func(m *nsmatch.NamespaceMatcher) {
				m.Swap(nonMatchingNS.Name, true) // previously matched
			},
			request:       nsRequest(nonMatchingNS.Name),
			wantResult:    reconcile.Result{},
			wantBroadcast: true,
		},
		{
			name: "state change: namespace transitions non-matching -> matching, broadcast",
			buildCache: func(t *testing.T) cache.Cache {
				mc := mock.NewCache(t)
				mc.OnGetSetNamespace(matchingNS.Labels).Return(nil)
				return mc
			},
			preloadState: func(m *nsmatch.NamespaceMatcher) {
				m.Swap(matchingNS.Name, false) // previously did not match
			},
			request:       nsRequest(matchingNS.Name),
			wantResult:    reconcile.Result{},
			wantBroadcast: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := nsmatch.NewNamespaceMatcher(sel, "")
			if tt.preloadState != nil {
				tt.preloadState(notifier)
			}

			r := &reconciler{
				cache:           tt.buildCache(t),
				nsMatchNotifier: notifier,
			}
			sub := r.nsMatchNotifier.Subscribe()
			result, err := r.doReconcile(t.Context(), logr.Discard(), tt.request)

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
		})
	}
}

func TestNsInitRunnable_Start(t *testing.T) {
	sel := labels.SelectorFromSet(labels.Set{"managed": "true"})

	matchingNS := makeNamespace("matching-ns", map[string]string{"managed": "true"})
	nonMatchingNS := makeNamespace("non-matching-ns", map[string]string{"managed": "false"})

	tests := []struct {
		name        string
		buildClient func() k8s.Client
		wantErr     bool
		wantStates  map[string]bool // ns name -> expected match state after Start
	}{
		{
			name: "list error is propagated",
			buildClient: func() k8s.Client {
				return k8s.NewFakeClientBuilder().
					WithInterceptorFuncs(interceptor.Funcs{
						List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
							return errors.New("list failed")
						},
					}).
					Build()
			},
			wantErr: true,
		},
		{
			name:        "no namespaces: notifier state remains empty",
			buildClient: func() k8s.Client { return k8s.NewFakeClient() },
			wantStates:  map[string]bool{},
		},
		{
			name:        "matching namespace is seeded as matching",
			buildClient: func() k8s.Client { return k8s.NewFakeClient(matchingNS) },
			wantStates:  map[string]bool{matchingNS.Name: true},
		},
		{
			name:        "non-matching namespace is seeded as non-matching",
			buildClient: func() k8s.Client { return k8s.NewFakeClient(nonMatchingNS) },
			wantStates:  map[string]bool{nonMatchingNS.Name: false},
		},
		{
			name:        "mixed namespaces: each seeded with correct match state",
			buildClient: func() k8s.Client { return k8s.NewFakeClient(matchingNS, nonMatchingNS) },
			wantStates: map[string]bool{
				matchingNS.Name:    true,
				nonMatchingNS.Name: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := nsmatch.NewNamespaceMatcher(sel, "")
			r := &namespaceSeedRunnable{
				client:           tt.buildClient(),
				namespaceMatcher: notifier,
			}

			err := r.Start(t.Context())

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Start launches a goroutine; poll until all expected states are visible.
			require.Eventually(t, func() bool {
				for ns, wantMatch := range tt.wantStates {
					if notifier.Matches(ns) != wantMatch {
						return false
					}
				}
				return true
			}, time.Second, time.Millisecond)

			for ns, wantMatch := range tt.wantStates {
				wasMatching := notifier.Swap(ns, false)
				assert.Equal(t, wantMatch, wasMatching, "unexpected match state for namespace %q", ns)
			}
		})
	}
}

func TestReconciler_Reconcile(t *testing.T) {
	sel := labels.SelectorFromSet(labels.Set{"managed": "true"})
	matchingNS := makeNamespace("matching-ns", map[string]string{"managed": "true"})

	tests := []struct {
		name       string
		lc         license.Checker
		wantResult reconcile.Result
		wantErr    bool
		wantGet    bool
	}{
		{
			name:    "license check error: error returned",
			lc:      errLicenseChecker{err: errors.New("license check failed")},
			wantErr: true,
		},
		{
			name:       "enterprise disabled: requeue after 5m, no reconcile",
			lc:         license.MockLicenseChecker{EnterpriseEnabled: false},
			wantResult: reconcile.Result{RequeueAfter: 5 * time.Minute},
		},
		{
			name:       "enterprise enabled: delegates to doReconcile",
			lc:         license.MockLicenseChecker{EnterpriseEnabled: true},
			wantResult: reconcile.Result{},
			wantGet:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := events.NewFakeRecorder(10)
			mc := mock.NewCache(t)
			if tt.wantGet {
				mc.OnGetSetNamespace(matchingNS.Labels).Return(nil)
			}
			r := &reconciler{
				cache:           mc,
				nsMatchNotifier: nsmatch.NewNamespaceMatcher(sel, ""),
				licenseChecker:  tt.lc,
				recorder:        fr,
			}

			result, err := r.Reconcile(t.Context(), nsRequest(matchingNS.Name))

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantResult, result)

			if ok, _ := r.licenseChecker.EnterpriseFeaturesEnabled(t.Context()); !ok && !tt.wantErr {
				assert.Equal(t, "Warning InvalidLicense Dynamic namespace selector is an enterprise feature. Enterprise features are disabled", <-fr.Events)
			}
		})
	}
}
