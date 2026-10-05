// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package namespace_selector

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/kibana"
)

// TestNamespaceSelectorDynamicLabelChange verifies that the operator dynamically picks up a namespace
// when it gains the label matched by the namespace selector, without requiring an operator restart.
//
// The test requires an enterprise license. It labels only ns1 at startup, configures the operator
// with a matchLabels selector for "eck-visible=true", waits for every operator Pod to run with it, confirms
// ES in ns1 is reconciled and ES in ns2 is ignored, then adds the label to ns2 and asserts the operator
// reconciles ES in ns2 without restarting.
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

	var expectedConfig map[string]any
	var leaderIdentity string

	// Always restore namespace labels and operator config on exit, even on test failure.
	registerNamespaceSelectorCleanup(t, k, eckVisibleLabel, originalConfig, ns1, ns2)

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
			Name: "switch operator to eck-visible=true namespace-selector",
			Test: func(t *testing.T) {
				require.NoError(t, helper.UpdateOperatorConfig(k.Client, func(cfg map[string]any) {
					delete(cfg, operator.NamespacesFlag)
					cfg[operator.NamespaceSelectorFlag] = map[string]any{
						"matchLabels": map[string]any{
							eckVisibleLabel: "true",
						},
					}
				}))
				expectedConfig, err = helper.GetOperatorConfig(k.Client)
				require.NoError(t, err)
			},
		}).
		WithStep(test.Step{
			Name: "wait for every operator Pod to run with the new namespace-selector config",
			Test: func(t *testing.T) {
				waitForOperatorConfig(t, t.Context(), k, expectedConfig)
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
		WithStep(recordOperatorLeader(k, &leaderIdentity)).
		WithStep(test.Step{
			Name: "add eck-visible=true to ns2 to trigger dynamic namespace pickup",
			Test: func(t *testing.T) {
				require.NoError(t, helper.SetNamespaceLabel(t.Context(), k.Client, ns2, eckVisibleLabel, "true"))
			},
		}).
		WithSteps(test.CheckTestSteps(esNs2, k)).
		WithStep(assertOperatorLeaderUnchanged(k, &leaderIdentity, "operator must not restart when a namespace gains the selector label")).
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

	var expectedConfig map[string]any
	var leaderIdentity string

	// Always restore namespace labels and operator config on exit, even on test failure.
	registerNamespaceSelectorCleanup(t, k, eckVisibleLabel, originalConfig, esNamespace, kbNamespace)

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
			Name: "switch operator to eck-visible=true namespace-selector",
			Test: func(t *testing.T) {
				require.NoError(t, helper.UpdateOperatorConfig(k.Client, func(cfg map[string]any) {
					delete(cfg, operator.NamespacesFlag)
					cfg[operator.NamespaceSelectorFlag] = map[string]any{
						"matchLabels": map[string]any{
							eckVisibleLabel: "true",
						},
					}
				}))
				expectedConfig, err = helper.GetOperatorConfig(k.Client)
				require.NoError(t, err)
			},
		}).
		WithStep(test.Step{
			Name: "wait for every operator Pod to run with the new namespace-selector config",
			Test: func(t *testing.T) {
				waitForOperatorConfig(t, t.Context(), k, expectedConfig)
			},
		}).
		WithSteps(esBuilder.InitTestSteps(k)).
		WithSteps(esBuilder.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(esBuilder, k)).
		WithSteps(kbBuilder.InitTestSteps(k)).
		WithSteps(kbBuilder.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(kbBuilder, k)).
		WithStep(kbAssociationStatusIs(commonv1.AssociationEstablished)).
		WithStep(recordOperatorLeader(k, &leaderIdentity)).
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
		WithStep(assertOperatorLeaderUnchanged(k, &leaderIdentity, "operator must not restart when namespaces flip in and out of the selector scope")).
		WithSteps(kbBuilder.DeletionTestSteps(k)).
		WithSteps(esBuilder.DeletionTestSteps(k)).
		WithStep(licenseTestContext.DeleteAllEnterpriseLicenseSecrets()).
		RunSequential(t)
}

// registerNamespaceSelectorCleanup registers a t.Cleanup that restores the namespace labels and operator
// config on test exit, even on test failure. It removes `labelToDelete` from the given namespaces, restores
// originalConfig and waits for every operator Pod to run with the restored config.
func registerNamespaceSelectorCleanup(t *testing.T, k *test.K8sClient, labelToDelete string, originalConfig map[string]any, namespaces ...string) {
	t.Helper()
	t.Cleanup(func() {
		// restore original config
		logf.Log.Info("Restoring operator config")
		test.Eventually(func() error {
			if err := helper.SetOperatorConfig(context.Background(), k.Client, originalConfig); err != nil {
				logf.Log.Error(err, "failed to restore operator config, retrying...")
				return err
			}
			logf.Log.Info("operator config was restored")
			return nil
		})(t)

		// Ensure that every operator Pod restarted with the restored config.
		logf.Log.Info("Waiting for every operator Pod to run with the restored config")
		// t.Context() is already canceled when cleanup functions run.
		waitForOperatorConfig(t, context.Background(), k, originalConfig)

		// Clean up the namespace labels only after the operator config has been successfully restored,
		// so the operator is back to its original (non namespace-selector) configuration before the
		// labels these tests rely on are removed.
		if err := helper.DeleteNamespaceLabel(context.Background(), k.Client, labelToDelete, namespaces...); err != nil {
			logf.Log.Error(err, "failed to delete namespaces labels")
		}
	})
}

// waitForOperatorConfig waits for every operator Pod to run with the namespace-related settings of expectedConfig.
// Waiting for all replicas, not just one, matters when the operator runs multiple replicas (e.g. the chaos job scales
// the operator up): kubelets refresh the ConfigMap volume on each node independently, so a standby replica still
// running with the previous config could otherwise acquire the leader lease and reconcile with it.
func waitForOperatorConfig(t *testing.T, ctx context.Context, k *test.K8sClient, expectedConfig map[string]any) {
	t.Helper()
	test.UntilSuccess(
		func() error {
			return helper.CheckOperatorPodsLoadedConfig(ctx, k, expectedConfig, operator.NamespacesFlag, operator.NamespaceSelectorFlag)
		},
		// kubelet ConfigMap propagation + file watcher poll (15s) + pod recreation
		3*time.Minute,
	)(t)
}

// recordOperatorLeader returns a step recording the identity of the current operator leader into leaderIdentity. It
// waits for the leader election Lease to be live, so that the identity of a terminated leader is never recorded.
func recordOperatorLeader(k *test.K8sClient, leaderIdentity *string) test.Step {
	return test.Step{
		Name: "record the operator leader identity",
		Test: func(t *testing.T) {
			test.Eventually(func() error {
				identity, live, err := helper.OperatorLeader(t.Context(), k)
				if err != nil {
					return err
				}
				if !live {
					return fmt.Errorf("operator leader election Lease held by %s is not live", identity)
				}
				*leaderIdentity = identity
				return nil
			})(t)
		},
	}
}

// assertOperatorLeaderUnchanged returns a step asserting that the operator leader election Lease is still held by the
// leader recorded in leaderIdentity. As only the leader reconciles resources and a new leader acquires the Lease with a
// new identity, an unchanged holder proves that the namespace scope changes in between were handled by the operator
// process recorded before them, i.e. without an operator restart.
func assertOperatorLeaderUnchanged(k *test.K8sClient, leaderIdentity *string, msg string) test.Step {
	return test.Step{
		Name: "assert the operator leader did not change, i.e. the operator did not restart",
		Test: func(t *testing.T) {
			var identity string
			test.Eventually(func() error {
				var err error
				identity, _, err = helper.OperatorLeader(t.Context(), k)
				return err
			})(t)
			if identity != *leaderIdentity && test.Ctx().DeployChaosJob {
				// the chaos job randomly deletes operator Pods and scales the operator, which replaces the leader
				// independently of the namespace-selector behaviour under test, making the result inconclusive.
				t.Skipf("operator leader changed from %s to %s, likely because of the chaos job", *leaderIdentity, identity)
			}
			require.Equal(t, *leaderIdentity, identity, msg)
		},
	}
}
