// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package namespace_selector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
)

// TestNamespaceSelectorDynamicLabelChange verifies that the operator dynamically picks up a namespace
// when it gains the label matched by the namespace selector, without requiring an operator restart.
//
// The test requires an enterprise license. It labels only ns1 at startup, configures the operator
// with a matchLabels selector for "eck-visible=true", waits for the restart, confirms ES in ns1 is
// reconciled and ES in ns2 is ignored, then adds the label to ns2 and asserts the operator reconciles
// ES in ns2 without restarting.
//
// NOTE: this test mutates global operator configuration and must not run in parallel
// with other tests in the same test run.
func TestNamespaceSelectorDynamicLabelChange(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	k := test.NewK8sClientOrFatal()
	ns1 := test.Ctx().ManagedNamespace(0)
	ns2 := test.Ctx().ManagedNamespace(1)

	const eckVisibleLabel = "eck-visible"

	licenseBytes, err := os.ReadFile(test.Ctx().TestLicense)
	require.NoError(t, err)

	esNs1 := elasticsearch.NewBuilder("ns-sel-dyn").
		WithNamespace(ns1).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	esNs2 := elasticsearch.NewBuilder("ns-sel-dyn-ns2").
		WithNamespace(ns2).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	licenseTestContext := elasticsearch.NewLicenseTestContext(k, esNs1.Elasticsearch)

	originalNamespaces, err := helper.GetOperatorConfigValue(k.Client, "namespaces")
	require.NoError(t, err)

	var restartCount int32

	// Always restore namespace labels and operator config on exit, even on test failure.
	t.Cleanup(func() {
		for _, nsName := range []string{ns1, ns2} {
			var ns corev1.Namespace
			if err := k.Client.Get(context.Background(), types.NamespacedName{Name: nsName}, &ns); err == nil {
				delete(ns.Labels, eckVisibleLabel)
				_ = k.Client.Update(context.Background(), &ns)
			}
		}
		if err := helper.UpdateOperatorConfig(k.Client, func(cfg map[string]any) {
			delete(cfg, "namespace-selector")
			if originalNamespaces != nil {
				cfg["namespaces"] = originalNamespaces
			}
		}); err != nil {
			t.Logf("WARNING: failed to restore operator config: %v", err)
			return
		}
		test.Eventually(func() error {
			newCount, err := helper.OperatorRestartCount(k)
			if err != nil {
				return err
			}
			if newCount <= restartCount {
				return errors.New("waiting to restart after config restore")
			}
			return nil
		})(t)
	})

	test.StepList{}.
		WithStep(licenseTestContext.DeleteAllEnterpriseLicenseSecrets()).
		WithStep(licenseTestContext.CreateEnterpriseLicenseSecret("eck-license-ns-sel-dynamic", licenseBytes)).
		WithStep(test.Step{
			Name: "add eck-visible=true label to ns1; ns2 remains unlabeled",
			Test: func(t *testing.T) {
				var ns corev1.Namespace
				require.NoError(t, k.Client.Get(context.Background(), types.NamespacedName{Name: ns1}, &ns))
				if ns.Labels == nil {
					ns.Labels = make(map[string]string)
				}
				ns.Labels[eckVisibleLabel] = "true"
				require.NoError(t, k.Client.Update(context.Background(), &ns))
			},
		}).
		WithStep(test.Step{
			Name: "record baseline operator restart count",
			Test: func(t *testing.T) {
				restartCount, err = helper.OperatorRestartCount(k)
				require.NoError(t, err)
			},
		}).
		WithStep(test.Step{
			Name: "switch operator to eck-visible=true namespace-selector",
			Test: func(t *testing.T) {
				require.NoError(t, helper.UpdateOperatorConfig(k.Client, func(cfg map[string]any) {
					delete(cfg, "namespaces")
					cfg["namespace-selector"] = map[string]any{
						"matchLabels": map[string]any{
							eckVisibleLabel: "true",
						},
					}
				}))
			},
		}).
		WithStep(test.Step{
			Name: "wait for operator restart with new namespace-selector config",
			Test: test.Eventually(func() error {
				newCount, err := helper.OperatorRestartCount(k)
				if err != nil {
					return err
				}
				if newCount <= restartCount {
					return fmt.Errorf("waiting for operator restart after namespace-selector config change (current restarts: %d)", newCount)
				}
				return nil
			}),
		}).
		WithStep(test.Step{
			Name: "record post-restart count as the no-restart baseline",
			Test: func(t *testing.T) {
				restartCount, err = helper.OperatorRestartCount(k)
				require.NoError(t, err)
			},
		}).
		WithSteps(esNs1.InitTestSteps(k)).
		WithSteps(esNs1.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(esNs1, k)).
		WithSteps(esNs2.InitTestSteps(k)).
		WithSteps(esNs2.CreationTestSteps(k)). // create the CRD but don't attempt to verify the cluster
		WithStep(test.Step{
			Name: "verify operator does not reconcile ES in the unlabeled ns2",
			Test: func(t *testing.T) {
				time.Sleep(30 * time.Second)
				require.NoError(t, k.CheckPodCount(0, test.ESPodListOptions(ns2, esNs2.Elasticsearch.Name)...))
			},
		}).
		WithStep(test.Step{
			Name: "add eck-visible=true to ns2 to trigger dynamic namespace pickup",
			Test: func(t *testing.T) {
				var ns corev1.Namespace
				require.NoError(t, k.Client.Get(context.Background(), types.NamespacedName{Name: ns2}, &ns))
				if ns.Labels == nil {
					ns.Labels = make(map[string]string)
				}
				ns.Labels[eckVisibleLabel] = "true"
				require.NoError(t, k.Client.Update(context.Background(), &ns))
			},
		}).
		WithSteps(test.CheckTestSteps(esNs2, k)).
		WithStep(test.Step{
			Name: "assert operator did not restart to pick up the newly labeled namespace",
			Test: func(t *testing.T) {
				postLabelRestartCount, err := helper.OperatorRestartCount(k)
				require.NoError(t, err)
				require.Equal(t, restartCount, postLabelRestartCount, "operator must not restart when a namespace gains the selector label")
			},
		}).
		WithSteps(esNs1.DeletionTestSteps(k)).
		WithSteps(esNs2.DeletionTestSteps(k)).
		WithStep(licenseTestContext.DeleteAllEnterpriseLicenseSecrets()).
		RunSequential(t)
}
