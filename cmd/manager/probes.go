// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// setupProbes configures the manager's liveness and readiness checks.
// Liveness reports whether the process can serve requests. Readiness is delayed
// until the local cache has synced and, when enabled, the webhook server has
// started. Cache readiness runs on every replica because non-leaders may serve
// webhook requests.
func setupProbes(mgr manager.Manager, startupCh <-chan struct{}, webhookEnabled bool) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("cache", func(req *http.Request) error {
		// Wait 2 second, we want to return a feedback to the kubelet in timely fashion.
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()

		// wait for cache to be started first (signaled by closed startupCh)
		select {
		case <-startupCh:
		case <-ctx.Done():
			return errors.New("cache not started yet")
		}

		// check that every registered informer is synced
		if !mgr.GetCache().WaitForCacheSync(ctx) {
			return errors.New("cache not synced")
		}
		return nil
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
