// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package namespace_selector

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/kibana"
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

	originalConfig, err := helper.GetOperatorConfig(k.Client)
	require.NoError(t, err)

	var restartCount int32

	// Always restore namespace labels and operator config on exit, even on test failure.
	registerNamespaceSelectorCleanup(t, k, eckVisibleLabel, originalConfig, &restartCount, ns1, ns2)

	test.StepList{}.
		WithStep(licenseTestContext.DeleteAllEnterpriseLicenseSecrets()).
		WithStep(licenseTestContext.CreateEnterpriseLicenseSecret("eck-license-ns-sel-dynamic", licenseBytes)).
		WithStep(test.Step{
			Name: "add eck-visible=true label to ns1; ns2 remains unlabeled",
			Test: func(t *testing.T) {
				require.NoError(t, helper.SetNamespaceLabel(t.Context(), k.Client, ns1, eckVisibleLabel, "true"))
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
			Test: waitForOperatorRestart(k, &restartCount, 30*time.Second),
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
				require.NoError(t, helper.SetNamespaceLabel(t.Context(), k.Client, ns2, eckVisibleLabel, "true"))
			},
		}).
		WithSteps(test.CheckTestSteps(esNs2, k)).
		WithStep(test.Step{
			Name: "assert operator did not restart to pick up the newly labeled namespace",
			Skip: func() bool {
				// the chaos job randomly deletes operator Pods and flips replica counts, which
				// resets/bumps restart counts independently of the namespace-selector behaviour
				// under test, making this assertion meaningless (and flaky) in that mode.
				return test.Ctx().DeployChaosJob
			},
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

// TestNamespaceSelectorDynamicLabelChangeAssociation verifies that a cross-namespace association follows
// the referenced resource's namespace in and out of the operator's namespace-selector scope.
//
// Both namespaces are labeled at startup, an Elasticsearch is created in ns1 and a Kibana
// referencing it in ns2, and the association must be Established. When ns1 (the Elasticsearch
// namespace) loses the label, the operator must stop seeing the referenced Elasticsearch and move
// the Kibana association to Pending. When ns1 is labeled again, the association must be
// re-established, all without an operator restart.
//
// The test requires an enterprise license.
//
// NOTE: this test mutates global operator configuration and must not run in parallel
// with other tests in the same test run.
func TestNamespaceSelectorDynamicLabelChangeAssociation(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	k := test.NewK8sClientOrFatal()
	esNamespace := test.Ctx().ManagedNamespace(1) // this namespace will be off-boarder later so use the second once since the license is installed in the first one.
	kbNamespace := test.Ctx().ManagedNamespace(0)

	const eckVisibleLabel = "eck-visible"

	licenseBytes, err := os.ReadFile(test.Ctx().TestLicense)
	require.NoError(t, err)

	esBuilder := elasticsearch.NewBuilder("ns-sel-assoc").
		WithNamespace(esNamespace).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)
	kbBuilder := kibana.NewBuilder("ns-sel-assoc").
		WithNamespace(kbNamespace).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	licenseTestContext := elasticsearch.NewLicenseTestContext(k, esBuilder.Elasticsearch)

	originalConfig, err := helper.GetOperatorConfig(k.Client)
	require.NoError(t, err)

	var restartCount int32

	// Always restore namespace labels and operator config on exit, even on test failure.
	registerNamespaceSelectorCleanup(t, k, eckVisibleLabel, originalConfig, &restartCount, esNamespace, kbNamespace)

	// kbAssociationStatusIs returns a step waiting for the Kibana Elasticsearch association
	// status to reach the expected value.
	kbAssociationStatusIs := func(expected commonv1.AssociationStatus) test.Step {
		return test.Step{
			Name: fmt.Sprintf("wait for Kibana Elasticsearch association status to be %q", expected),
			Test: test.Eventually(func() error {
				var kb kbv1.Kibana
				if err := k.Client.Get(t.Context(), types.NamespacedName{
					Namespace: kbBuilder.Kibana.Namespace,
					Name:      kbBuilder.Kibana.Name,
				}, &kb); err != nil {
					return err
				}
				if s := kb.Status.ElasticsearchAssociationStatus; s != expected {
					return fmt.Errorf("kibana Elasticsearch association status is %q, expected %q", s, expected)
				}
				return nil
			}),
		}
	}

	test.StepList{}.
		WithStep(licenseTestContext.DeleteAllEnterpriseLicenseSecrets()).
		WithStep(licenseTestContext.CreateEnterpriseLicenseSecret("eck-license-ns-sel-assoc", licenseBytes)).
		WithStep(test.Step{
			Name: "add eck-visible=true label to both the Elasticsearch and the Kibana namespaces",
			Test: func(t *testing.T) {
				require.NoError(t, helper.SetNamespaceLabel(t.Context(), k.Client, esNamespace, eckVisibleLabel, "true"))
				require.NoError(t, helper.SetNamespaceLabel(t.Context(), k.Client, kbNamespace, eckVisibleLabel, "true"))
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
			Test: waitForOperatorRestart(k, &restartCount, 30*time.Second),
		}).
		WithStep(test.Step{
			Name: "record post-restart count as the no-restart baseline",
			Test: func(t *testing.T) {
				restartCount, err = helper.OperatorRestartCount(k)
				require.NoError(t, err)
			},
		}).
		WithSteps(esBuilder.InitTestSteps(k)).
		WithSteps(esBuilder.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(esBuilder, k)).
		WithSteps(kbBuilder.InitTestSteps(k)).
		WithSteps(kbBuilder.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(kbBuilder, k)).
		WithStep(kbAssociationStatusIs(commonv1.AssociationEstablished)).
		WithStep(test.Step{
			Name: "remove the eck-visible label from the Elasticsearch namespace",
			Test: func(t *testing.T) {
				require.NoError(t, helper.DeleteNamespaceLabel(t.Context(), k.Client, eckVisibleLabel, esNamespace))
			},
		}).
		WithStep(kbAssociationStatusIs(commonv1.AssociationPending)).
		WithStep(test.Step{
			Name: "re-add eck-visible=true to the Elasticsearch namespace",
			Test: func(t *testing.T) {
				require.NoError(t, helper.SetNamespaceLabel(t.Context(), k.Client, esNamespace, eckVisibleLabel, "true"))
			},
		}).
		WithStep(kbAssociationStatusIs(commonv1.AssociationEstablished)).
		WithStep(test.Step{
			Name: "assert operator did not restart during the namespace scope changes",
			Skip: func() bool {
				// the chaos job randomly deletes operator Pods and flips replica counts, which
				// resets/bumps restart counts independently of the namespace-selector behaviour
				// under test, making this assertion meaningless (and flaky) in that mode.
				return test.Ctx().DeployChaosJob
			},
			Test: func(t *testing.T) {
				postLabelRestartCount, err := helper.OperatorRestartCount(k)
				require.NoError(t, err)
				require.Equal(t, restartCount, postLabelRestartCount, "operator must not restart when namespaces flip in and out of the selector scope")
			},
		}).
		WithSteps(kbBuilder.DeletionTestSteps(k)).
		WithSteps(esBuilder.DeletionTestSteps(k)).
		WithStep(licenseTestContext.DeleteAllEnterpriseLicenseSecrets()).
		RunSequential(t)
}

// registerNamespaceSelectorCleanup registers a t.Cleanup that restores the namespace labels and operator
// config on test exit, even on test failure. It removes `labelToDelete` from the given namespaces, restores
// originalConfig and waits for the operator to restart to pick up the restored config. restartCount is
// dereferenced at cleanup time, so it must point to the latest recorded pre-restore restart count.
func registerNamespaceSelectorCleanup(t *testing.T, k *test.K8sClient, labelToDelete string, originalConfig map[string]any, restartCount *int32, namespaces ...string) {
	t.Helper()
	t.Cleanup(func() {
		// restore original config
		test.Eventually(func() error {
			if err := helper.SetOperatorConfig(k.Client, originalConfig); err != nil {
				t.Logf("WARNING: failed to restore operator config: %v", err)
				return err
			}
			return nil
		})(t)

		// Ensure that the operator restarts.
		waitForOperatorRestart(k, restartCount, 1*time.Minute)(t)

		// Clean up the namespace labels only after the operator config has been successfully restored,
		// so the operator is back to its original (non namespace-selector) configuration before the
		// labels these tests rely on are removed.
		if err := helper.DeleteNamespaceLabel(t.Context(), k.Client, labelToDelete, namespaces...); err != nil {
			t.Logf("WARNING: failed to delete namespaces labels: %s", err.Error())
		}
	})
}

// waitForOperatorRestart waits for the operator to restart by checking restart count of pod. [chaosSleepDuration] is used
// only when chaos job is deployed.
func waitForOperatorRestart(k *test.K8sClient, restartCount *int32, chaosSleepDuration time.Duration) func(*testing.T) {
	return func(t *testing.T) {
		test.Eventually(func() error {
			if test.Ctx().DeployChaosJob {
				// In chaos mode restart counting is unreliable, so we cannot wait for a restart-count increment.
				// Instead just wait and hope the ECK operator has restarted.
				time.Sleep(chaosSleepDuration)
				return nil
			}

			newCount, err := helper.OperatorRestartCount(k)
			if err != nil {
				return err
			}
			if newCount <= *restartCount {
				return fmt.Errorf("waiting for operator restart after (current restarts: %d)", newCount)
			}
			return nil
		})(t)
	}
}
