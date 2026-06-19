// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package namespace_selector

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
)

// TestNamespaceSelector verifies that the operator correctly manages resources when configured
// with a namespace label selector instead of an explicit managed-namespaces list.
// It reconfigures the operator at runtime, waits for the resulting restart, and then
// verifies that resources in a namespace matched by the selector are reconciled.
//
// NOTE: this test mutates global operator configuration and must not run in parallel
// with other tests in the same test run.
func TestNamespaceSelector(t *testing.T) {
	k := test.NewK8sClientOrFatal()
	ns1 := test.Ctx().ManagedNamespace(0)
	ns2 := test.Ctx().ManagedNamespace(1)

	originalNamespaces, err := helper.GetOperatorConfigValue(k.Client, "namespaces")
	require.NoError(t, err)

	var restartCount int32

	t.Cleanup(func() {
		// restore original operator config: remove namespace-selector, put back namespaces
		err := helper.UpdateOperatorConfig(k.Client, func(cfg map[string]any) {
			delete(cfg, "namespace-selector")
			if originalNamespaces != nil {
				cfg["namespaces"] = originalNamespaces
			}
		})
		if err != nil {
			t.Logf("WARNING: failed to restore operator config: %v", err)
			return
		}
		test.Eventually(func() error {
			newCount, err := helper.OperatorRestartCount(k)
			if err != nil {
				return err
			}
			if newCount <= restartCount {
				return errors.New("waiting to restart after config restore") // not yet restarted after cleanup
			}
			return nil
		})(t)
	})

	// step 1: record current restart count before reconfiguring
	restartCount, err = helper.OperatorRestartCount(k)
	require.NoError(t, err)

	// step 2: switch to namespace-selector mode — the test namespaces are already
	// labeled with test-run=<testRun> by the e2e setup (config/e2e/managed_namespaces.yaml)
	require.NoError(t, helper.UpdateOperatorConfig(k.Client, func(cfg map[string]any) {
		delete(cfg, "namespaces")
		cfg["namespace-selector"] = map[string]any{
			"matchExpressions": []map[string]any{
				{
					"key":      "kubernetes.io/metadata.name",
					"operator": "In",
					"values":   []string{ns1},
				},
			},
		}
	}))

	// step 3: wait for operator to restart and pick up the new config
	test.Eventually(func() error {
		newCount, err := helper.OperatorRestartCount(k)
		if err != nil {
			return err
		}
		if newCount <= restartCount {
			return fmt.Errorf("waiting for operator restart after namespace-selector config change (current restarts: %d)", newCount)
		}
		return nil
	})(t)

	// update restart count so the cleanup waiter starts from the right baseline
	restartCount, err = helper.OperatorRestartCount(k)
	require.NoError(t, err)

	// step 4: verify the operator manages a resource in the selector-matched namespace
	es := elasticsearch.NewBuilder("ns-selector").
		WithNamespace(ns1).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	test.Sequence(nil, test.EmptySteps, es).RunSequential(t)

	// step 5: verify an ES CR in a non-managed namespace is NOT reconciled
	esIgnored := elasticsearch.NewBuilder("ns-selector-ignored").
		WithNamespace(ns2).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	require.NoError(t, k.Client.Create(context.Background(), &esIgnored.Elasticsearch))
	t.Cleanup(func() {
		_ = k.Client.Delete(context.Background(), &esIgnored.Elasticsearch)
	})

	// The operator must not reconcile resources in namespaces not matched by the selector.
	// Give it enough time to act if it were going to, then assert no pods were created.
	time.Sleep(30 * time.Second)
	require.NoError(t, k.CheckPodCount(0, test.ESPodListOptions(ns2, esIgnored.Elasticsearch.Name)...))
}
