// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"
	crwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
)

type probeTestCache struct {
	cache.Cache
	synced bool
}

func (c *probeTestCache) WaitForCacheSync(context.Context) bool {
	return c.synced
}

type probeTestWebhook struct {
	crwebhook.Server
	checker healthz.Checker
}

func (w *probeTestWebhook) StartedChecker() healthz.Checker {
	return w.checker
}

type probeTestManager struct {
	ctrlmanager.Manager
	healthChecks map[string]healthz.Checker
	readyChecks  map[string]healthz.Checker
	runnables    []ctrlmanager.Runnable
	cache        cache.Cache
	webhook      crwebhook.Server
	healthErr    error
	readyErrors  map[string]error
	addErr       error
}

func newProbeTestManager() *probeTestManager {
	return &probeTestManager{
		healthChecks: make(map[string]healthz.Checker),
		readyChecks:  make(map[string]healthz.Checker),
		readyErrors:  make(map[string]error),
		cache:        &probeTestCache{synced: true},
		webhook: &probeTestWebhook{
			checker: healthz.Ping,
		},
	}
}

func (m *probeTestManager) AddHealthzCheck(name string, check healthz.Checker) error {
	if m.healthErr != nil {
		return m.healthErr
	}
	m.healthChecks[name] = check
	return nil
}

func (m *probeTestManager) AddReadyzCheck(name string, check healthz.Checker) error {
	if err := m.readyErrors[name]; err != nil {
		return err
	}
	m.readyChecks[name] = check
	return nil
}

func (m *probeTestManager) Add(runnable ctrlmanager.Runnable) error {
	if m.addErr != nil {
		return m.addErr
	}
	m.runnables = append(m.runnables, runnable)
	return nil
}

func (m *probeTestManager) GetCache() cache.Cache {
	return m.cache
}

func (m *probeTestManager) GetWebhookServer() crwebhook.Server {
	return m.webhook
}

func TestSetupProbes(t *testing.T) {
	setupErr := errors.New("setup error")
	tests := []struct {
		name             string
		webhookEnabled   bool
		configure        func(*probeTestManager)
		wantErrContains  string
		wantHealthChecks []string
		wantReadyChecks  []string
	}{
		{
			name:             "registers cache and health checks",
			wantHealthChecks: []string{"healthz"},
			wantReadyChecks:  []string{"cache"},
		},
		{
			name:             "registers webhook check when enabled",
			webhookEnabled:   true,
			wantHealthChecks: []string{"healthz"},
			wantReadyChecks:  []string{"cache", "webhook"},
		},
		{
			name: "propagates health check registration error",
			configure: func(m *probeTestManager) {
				m.healthErr = setupErr
			},
			wantErrContains: "failed to set up health check",
		},
		{
			name: "propagates cache check registration error",
			configure: func(m *probeTestManager) {
				m.readyErrors["cache"] = setupErr
			},
			wantErrContains:  "failed to set up cache readiness check",
			wantHealthChecks: []string{"healthz"},
		},
		{
			name:           "propagates webhook check registration error",
			webhookEnabled: true,
			configure: func(m *probeTestManager) {
				m.readyErrors["webhook"] = setupErr
			},
			wantErrContains:  "failed to set up webhook readiness check",
			wantHealthChecks: []string{"healthz"},
			wantReadyChecks:  []string{"cache"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newProbeTestManager()
			if tt.configure != nil {
				tt.configure(mgr)
			}

			startupCh := make(chan struct{})
			err := setupProbes(mgr, startupCh, tt.webhookEnabled)

			if tt.wantErrContains == "" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, setupErr)
				require.ErrorContains(t, err, tt.wantErrContains)
			}
			require.ElementsMatch(t, tt.wantHealthChecks, mapKeys(mgr.healthChecks))
			require.ElementsMatch(t, tt.wantReadyChecks, mapKeys(mgr.readyChecks))
			require.Empty(t, mgr.runnables)
		})
	}
}

func TestCacheReadinessProbe(t *testing.T) {
	tests := []struct {
		name             string
		startupCacheSync bool // controls whether startupCh is closed (startupRunnable's cache)
		mainCacheSync    bool // controls WaitForCacheSync result in the readiness check
		cancelCtx        bool
		wantErrContains  string
	}{
		{
			name:             "ready when cache started and all informers synced",
			startupCacheSync: true,
			mainCacheSync:    true,
		},
		{
			name:            "not ready when cache not started yet",
			cancelCtx:       true,
			wantErrContains: "cache not started yet",
		},
		{
			name:             "not ready when informers not synced after startup",
			startupCacheSync: true,
			wantErrContains:  "cache not synced",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newProbeTestManager()
			mgr.cache = &probeTestCache{synced: tt.mainCacheSync}
			startupCh := make(chan struct{})
			require.NoError(t, setupProbes(mgr, startupCh, false))

			runnable := &startupRunnable{cache: &probeTestCache{synced: tt.startupCacheSync}, readyCh: startupCh}
			require.NoError(t, runnable.Start(t.Context()))

			ctx, cancel := context.WithCancel(t.Context())
			if tt.cancelCtx {
				cancel()
			} else {
				defer cancel()
			}
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx)
			err := mgr.readyChecks["cache"](req)
			if tt.wantErrContains != "" {
				require.ErrorContains(t, err, tt.wantErrContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func mapKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
