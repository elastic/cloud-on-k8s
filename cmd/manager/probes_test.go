// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlmanager "sigs.k8s.io/controller-runtime/pkg/manager"
	crwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
)

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

func (m *probeTestManager) GetWebhookServer() crwebhook.Server {
	return m.webhook
}

func closedCh() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func TestCacheReadyRunnable(t *testing.T) {
	r := &cacheReadyRunnable{readyCh: make(chan struct{})}
	assert.False(t, r.NeedLeaderElection(), "must run on every replica, not only the leader")

	require.NoError(t, r.Start(t.Context()))

	select {
	case <-r.readyCh:
	default:
		t.Fatal("readyCh should be closed after Start")
	}
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

			err := setupProbes(mgr, closedCh(), tt.webhookEnabled)

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
		name            string
		cacheReady      bool
		wantErrContains string
	}{
		{
			name:       "ready when channel is closed",
			cacheReady: true,
		},
		{
			name:            "not ready when channel is open",
			wantErrContains: "cache not synced",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ch <-chan struct{}
			if tt.cacheReady {
				ch = closedCh()
			} else {
				ch = make(chan struct{})
			}
			mgr := newProbeTestManager()
			require.NoError(t, setupProbes(mgr, ch, false))

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil)
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
