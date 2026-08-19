// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var _ manager.LeaderElectionRunnable = (*cacheReadyRunnable)(nil)

// cacheReadyRunnable signals via readyCh that all pre-registered informers have synced.
// Informers are pre-registered in registerWebhookInformers and newLicenseCheckRunnable.
// NeedLeaderElection=false ensures it runs on every replica, including non-leaders that serve webhook traffic.
type cacheReadyRunnable struct{ readyCh chan struct{} }

func (r *cacheReadyRunnable) NeedLeaderElection() bool { return false }

func (r *cacheReadyRunnable) Start(_ context.Context) error {
	close(r.readyCh)
	return nil
}

// setupProbes configures the manager's liveness and readiness checks.
// Liveness reports whether the process can serve requests. Readiness is delayed
// until cacheReady is closed (signaled by cacheReadyRunnable) and, when enabled,
// the webhook server has started.
func setupProbes(mgr manager.Manager, cacheReady <-chan struct{}, webhookEnabled bool) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("cache", func(_ *http.Request) error {
		select {
		case <-cacheReady:
			return nil
		default:
			return errors.New("cache not synced")
		}
	}); err != nil {
		return fmt.Errorf("failed to set up cache readiness check: %w", err)
	}
	if webhookEnabled {
		if err := mgr.AddReadyzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
			return fmt.Errorf("failed to set up webhook readiness check: %w", err)
		}
	}
	return nil
}
