// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

const (
	// pinnedIndex is kept on the leaving nodes so that their remove-type shutdowns stay registered
	pinnedIndex = "pinned-to-leaving-nodes"
	// shutdownsKeptFor is how long the shutdowns of the leaving nodes must not be cancelled. Their Pods become not ready
	// about 15 seconds after the shutdowns are registered, which is when the downscale used to cancel them.
	shutdownsKeptFor = time.Minute
)

// TestDownscaleKeepsShutdownsWhileAnotherPodIsNotReady removes a NodeSet while the Pod of another NodeSet cannot be
// scheduled. The readiness probe makes the leaving nodes not ready as soon as their remove-type shutdown is registered,
// which must not make the downscale use up its budget and cancel the shutdowns it is waiting for.
func TestDownscaleKeepsShutdownsWhileAnotherPodIsNotReady(t *testing.T) {
	// nodes with a registered shutdown are only reported not ready by the readiness port, used after 8.2.0
	if version.MustParse(test.Ctx().ElasticStackVersion).LE(esv1.MinReadinessPortVersion) {
		t.SkipNow()
	}

	ctx := t.Context()
	k := test.NewK8sClientOrFatal()

	initial := elasticsearch.NewBuilder("test-downscale-keep-shutdowns").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(2, "data", elasticsearch.DefaultResources)

	// rename the data NodeSet, which removes the data nodes once their data is migrated to the new ones
	renamed := initial.WithNoESTopology().
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(2, "data-new", elasticsearch.DefaultResources)

	// add a NodeSet whose Pod cannot be scheduled, so that one Pod is not ready during the removal
	pendingNodeSet, fixedNodeSet := pendingNodeSets("pending", elasticsearch.DataRoleCfg(initial.Elasticsearch.Spec.Version))
	renamedWithPending := renamed.DeepCopy().WithNodeSet(pendingNodeSet)
	// eventually let the Pod be scheduled: a NodeSet whose Pods never joined the cluster cannot be removed
	fixed := renamed.DeepCopy().WithNodeSet(fixedNodeSet)

	esName := initial.Elasticsearch.Name
	esNamespace := initial.Elasticsearch.Namespace
	pendingPod := sset.PodName(esv1.StatefulSet(esName, "pending"), 0)
	leavingNodes := []string{
		sset.PodName(esv1.StatefulSet(esName, "data"), 0),
		sset.PodName(esv1.StatefulSet(esName, "data"), 1),
	}
	var esClient esclient.Client
	leavingPods := test.NewPodRestartChecker("leaving Elasticsearch", test.ESPodListOptionsByNodeSet(esNamespace, esName, "data")...)

	// one primary on each leaving node, without replicas: Elasticsearch considers a node holding only a replica that
	// cannot move as safe to remove
	pinnedIndexSettings := fmt.Sprintf(
		`{"settings": {"number_of_shards": 2, "number_of_replicas": 0, "index.routing.allocation.total_shards_per_node": 1, "index.routing.allocation.exclude._name": "%s-*"}}`,
		esv1.StatefulSet(esName, "data-new"),
	)

	test.StepList{}.
		WithSteps(initial.InitTestSteps(k)).
		WithSteps(initial.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(initial, k)).
		WithStep(createPinnedIndexStep(ctx, k, initial, &esClient, "the new data nodes", pinnedIndexSettings)).
		WithStep(elasticsearch.CheckClusterHealth(initial, k)).
		WithSteps(leavingPods.RecordUIDs(k)).
		WithSteps(renamedWithPending.UpgradeTestSteps(k)).
		WithSteps(shutdownsKeptSteps(ctx, k, initial, &esClient, leavingNodes, leavingPods)).
		WithStep(test.Step{
			Name: "Allow the index to be migrated to the new data nodes",
			Test: test.Eventually(func() error {
				_, err := elasticsearch.DoRequest(ctx, esClient, http.MethodPut, "/"+pinnedIndex+"/_settings",
					[]byte(`{"index.routing.allocation.exclude._name": null}`))
				return err
			}),
		}).
		WithStep(test.Step{
			Name: "Leaving nodes should be removed while the pending Pod is still not ready",
			// the removal must not wait for the pending Pod: stop retrying as soon as it is ready
			Test: test.RetryOnError(func() error {
				if err := checkPodsNotReady(k, esNamespace, []string{pendingPod}); err != nil {
					return err
				}
				var statefulSet appsv1.StatefulSet
				err := k.Client.Get(ctx, types.NamespacedName{Namespace: esNamespace, Name: esv1.StatefulSet(esName, "data")}, &statefulSet)
				if apierrors.IsNotFound(err) {
					return nil
				}
				if err != nil {
					return err
				}
				return fmt.Errorf("StatefulSet %s still exists with %d replicas", statefulSet.Name, statefulSet.Status.Replicas)
			}, func(err error) bool { return !errors.Is(err, errPodReady) }, test.Ctx().TestTimeout),
		}).
		WithSteps(fixed.UpgradeTestSteps(k)).
		WithSteps(test.CheckTestSteps(fixed, k)).
		WithSteps(fixed.DeletionTestSteps(k)).
		RunSequential(t)
}

// TestDownscaleRemovesDrainingMasterWhileAnotherPodIsNotReady removes a master node while the Pod of a data NodeSet cannot
// be scheduled. The not ready Pods use up the budget, but the draining master is the only unavailable master, so its
// removal does not consume the budget: it must be removed once drained, without waiting for the other Pod to be ready.
func TestDownscaleRemovesDrainingMasterWhileAnotherPodIsNotReady(t *testing.T) {
	// nodes with a registered shutdown are only reported not ready by the readiness port, used after 8.2.0
	if version.MustParse(test.Ctx().ElasticStackVersion).LE(esv1.MinReadinessPortVersion) {
		t.SkipNow()
	}

	ctx := t.Context()
	k := test.NewK8sClientOrFatal()

	initial := elasticsearch.NewBuilder("test-downscale-master-shutdown").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	downscaled := initial.WithNoESTopology().
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	// add a NodeSet whose Pod cannot be scheduled: with 3 desired nodes and maxUnavailable 1, the not ready leaving
	// master and the pending Pod use up the budget
	pendingNodeSet, fixedNodeSet := pendingNodeSets("pending", elasticsearch.DataRoleCfg(initial.Elasticsearch.Spec.Version))
	downscaledWithPending := downscaled.DeepCopy().WithNodeSet(pendingNodeSet)
	fixed := downscaled.DeepCopy().WithNodeSet(fixedNodeSet)

	esName := initial.Elasticsearch.Name
	esNamespace := initial.Elasticsearch.Namespace
	masterStatefulSet := esv1.StatefulSet(esName, "masterdata")
	leavingNode := sset.PodName(masterStatefulSet, 2)
	pendingPod := sset.PodName(esv1.StatefulSet(esName, "pending"), 0)
	var esClient esclient.Client
	masterPods := test.NewPodRestartChecker("master Elasticsearch", test.ESPodListOptionsByNodeSet(esNamespace, esName, "masterdata")...)

	// a single primary on the leaving master, without replicas
	pinnedIndexSettings := fmt.Sprintf(
		`{"settings": {"number_of_shards": 1, "number_of_replicas": 0, "index.routing.allocation.require._name": "%s"}}`,
		leavingNode,
	)

	test.StepList{}.
		WithSteps(initial.InitTestSteps(k)).
		WithSteps(initial.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(initial, k)).
		WithStep(createPinnedIndexStep(ctx, k, initial, &esClient, "the remaining master nodes", pinnedIndexSettings)).
		WithStep(elasticsearch.CheckClusterHealth(initial, k)).
		WithSteps(masterPods.RecordUIDs(k)).
		WithSteps(downscaledWithPending.UpgradeTestSteps(k)).
		WithSteps(shutdownsKeptSteps(ctx, k, initial, &esClient, []string{leavingNode}, masterPods)).
		WithStep(test.Step{
			Name: "Allow the index to be migrated to the remaining master nodes",
			Test: test.Eventually(func() error {
				_, err := elasticsearch.DoRequest(ctx, esClient, http.MethodPut, "/"+pinnedIndex+"/_settings",
					[]byte(`{"index.routing.allocation.require._name": null}`))
				return err
			}),
		}).
		WithStep(test.Step{
			Name: "Leaving master should be removed while the pending Pod is still not ready",
			// the removal must not wait for the pending Pod: stop retrying as soon as it is ready
			Test: test.RetryOnError(func() error {
				if err := checkPodsNotReady(k, esNamespace, []string{pendingPod}); err != nil {
					return err
				}
				var statefulSet appsv1.StatefulSet
				if err := k.Client.Get(ctx, types.NamespacedName{Namespace: esNamespace, Name: masterStatefulSet}, &statefulSet); err != nil {
					return err
				}
				if replicas := sset.GetReplicas(statefulSet); replicas != 2 {
					return fmt.Errorf("StatefulSet %s has %d replicas", masterStatefulSet, replicas)
				}
				return nil
			}, func(err error) bool { return !errors.Is(err, errPodReady) }, test.Ctx().TestTimeout),
		}).
		WithSteps(fixed.UpgradeTestSteps(k)).
		WithSteps(test.CheckTestSteps(fixed, k)).
		WithSteps(fixed.DeletionTestSteps(k)).
		RunSequential(t)
}

// TestDownscaleHoldsDrainingMasterWhileAnotherMasterIsNotReady removes a master node while the Pod of a new master
// NodeSet cannot be scheduled. Once its remove-type shutdown made it not ready, the draining master is one of two
// unavailable masters: its removal consumes the budget, which the new master uses up, so it must be held with its
// shutdown until the new master is ready.
func TestDownscaleHoldsDrainingMasterWhileAnotherMasterIsNotReady(t *testing.T) {
	// nodes with a registered shutdown are only reported not ready by the readiness port, used after 8.2.0
	if version.MustParse(test.Ctx().ElasticStackVersion).LE(esv1.MinReadinessPortVersion) {
		t.SkipNow()
	}

	ctx := t.Context()
	k := test.NewK8sClientOrFatal()

	initial := elasticsearch.NewBuilder("test-downscale-hold-master").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	downscaled := initial.WithNoESTopology().
		WithESMasterDataNodes(2, elasticsearch.DefaultResources)

	// add a master NodeSet whose Pod cannot be scheduled
	pendingNodeSet, fixedNodeSet := pendingNodeSets("pending-master", elasticsearch.MasterRoleCfg(initial.Elasticsearch.Spec.Version))
	downscaledWithPending := downscaled.DeepCopy().WithNodeSet(pendingNodeSet)
	fixed := downscaled.DeepCopy().WithNodeSet(fixedNodeSet)

	esName := initial.Elasticsearch.Name
	esNamespace := initial.Elasticsearch.Namespace
	masterStatefulSet := esv1.StatefulSet(esName, "masterdata")
	leavingNode := sset.PodName(masterStatefulSet, 2)
	var esClient esclient.Client
	masterPods := test.NewPodRestartChecker("master Elasticsearch", test.ESPodListOptionsByNodeSet(esNamespace, esName, "masterdata")...)

	// a single primary on the leaving master, without replicas: its shutdown can only complete once it is not ready,
	// otherwise the ready master would be removed within the budget
	pinnedIndexSettings := fmt.Sprintf(
		`{"settings": {"number_of_shards": 1, "number_of_replicas": 0, "index.routing.allocation.require._name": "%s"}}`,
		leavingNode,
	)

	test.StepList{}.
		WithSteps(initial.InitTestSteps(k)).
		WithSteps(initial.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(initial, k)).
		WithStep(createPinnedIndexStep(ctx, k, initial, &esClient, "the remaining master nodes", pinnedIndexSettings)).
		WithStep(elasticsearch.CheckClusterHealth(initial, k)).
		WithSteps(masterPods.RecordUIDs(k)).
		WithSteps(downscaledWithPending.UpgradeTestSteps(k)).
		WithSteps(shutdownsKeptSteps(ctx, k, initial, &esClient, []string{leavingNode}, masterPods)).
		WithStep(test.Step{
			Name: "Allow the index to be migrated to the remaining master nodes",
			Test: test.Eventually(func() error {
				_, err := elasticsearch.DoRequest(ctx, esClient, http.MethodPut, "/"+pinnedIndex+"/_settings",
					[]byte(`{"index.routing.allocation.require._name": null}`))
				return err
			}),
		}).
		WithStep(test.Step{
			Name: "Leaving master should have a complete remove-type shutdown",
			Test: test.Eventually(func() error {
				return checkRemoveShutdownsComplete(ctx, esClient, []string{leavingNode})
			}),
		}).
		WithStep(test.Step{
			// the master is drained: only the unavailable new master holds its removal
			Name: "Leaving master should be held while the new master is not ready",
			Test: func(t *testing.T) {
				t.Helper()
				require.Never(t, func() bool {
					err := checkRemoveShutdowns(ctx, esClient, []string{leavingNode})
					if err != nil {
						t.Log(err)
					}
					// errors while querying Elasticsearch are not considered as a removal
					return errors.Is(err, errNoRemoveShutdown)
				}, shutdownsKeptFor, 2*time.Second, "the leaving master was removed while the new master is not ready")
			},
		}).
		WithSteps(fixed.UpgradeTestSteps(k)).
		WithSteps(test.CheckTestSteps(fixed, k)).
		WithSteps(fixed.DeletionTestSteps(k)).
		RunSequential(t)
}

// TestDownscaleDuringRollingUpgrade removes data nodes while a pod template change is applied to all the NodeSets.
// The leaving nodes are not upgraded: their shutdowns are kept and their Pods are not restarted, while the staying data
// node is upgraded, then the master, which does not wait for the leaving nodes to be upgraded.
func TestDownscaleDuringRollingUpgrade(t *testing.T) {
	// nodes with a registered shutdown are only reported not ready by the readiness port, used after 8.2.0
	if version.MustParse(test.Ctx().ElasticStackVersion).LE(esv1.MinReadinessPortVersion) {
		t.SkipNow()
	}

	ctx := t.Context()
	k := test.NewK8sClientOrFatal()

	initial := elasticsearch.NewBuilder("test-downscale-rolling-upgrade").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(3, "data", elasticsearch.DefaultResources)
	// the not ready leaving nodes must not prevent the staying nodes from being upgraded
	initial.Elasticsearch.Spec.UpdateStrategy.ChangeBudget.MaxUnavailable = new(int32(3))

	mutated := initial.WithNoESTopology().
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(1, "data", elasticsearch.DefaultResources).
		WithEnvironmentVariable("E2E_DOWNSCALE_ROLLING_UPGRADE", "true")

	esName := initial.Elasticsearch.Name
	esNamespace := initial.Elasticsearch.Namespace
	dataStatefulSet := esv1.StatefulSet(esName, "data")
	leavingNodes := []string{sset.PodName(dataStatefulSet, 1), sset.PodName(dataStatefulSet, 2)}
	var esClient esclient.Client
	leavingPods := test.NewPodRestartChecker("leaving Elasticsearch", podListOptionsByName(t, esNamespace, leavingNodes...)...)
	stayingDataPod := test.NewPodRestartChecker("staying data Elasticsearch", podListOptionsByName(t, esNamespace, sset.PodName(dataStatefulSet, 0))...)
	masterPod := test.NewPodRestartChecker("master Elasticsearch", test.ESPodListOptionsByNodeSet(esNamespace, esName, "master")...)

	// a single primary on the last leaving node, without replicas: the leaving nodes are removed from the last one, so
	// both are held
	pinnedIndexSettings := fmt.Sprintf(
		`{"settings": {"number_of_shards": 1, "number_of_replicas": 0, "index.routing.allocation.require._name": "%s"}}`,
		leavingNodes[1],
	)

	test.StepList{}.
		WithSteps(initial.InitTestSteps(k)).
		WithSteps(initial.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(initial, k)).
		WithStep(createPinnedIndexStep(ctx, k, initial, &esClient, "the staying data node", pinnedIndexSettings)).
		WithStep(elasticsearch.CheckClusterHealth(initial, k)).
		WithSteps(leavingPods.RecordUIDs(k)).
		WithSteps(stayingDataPod.RecordUIDs(k)).
		WithSteps(masterPod.RecordUIDs(k)).
		WithSteps(mutated.UpgradeTestSteps(k)).
		WithSteps(shutdownsKeptSteps(ctx, k, initial, &esClient, leavingNodes, leavingPods)).
		WithSteps(stayingDataPod.WaitForRestart(k)).
		// the master is only upgraded once the non-master nodes are, which must not include the leaving ones
		WithSteps(masterPod.WaitForRestart(k)).
		WithStep(test.Step{
			Name: "Leaving nodes should still have a remove-type shutdown and not have been restarted",
			Test: test.Eventually(func() error {
				if err := checkRemoveShutdowns(ctx, esClient, leavingNodes); err != nil {
					return err
				}
				return leavingPods.CheckNotRestarted(ctx, k)
			}),
		}).
		WithStep(test.Step{
			Name: "Allow the index to be migrated to the staying data node",
			Test: test.Eventually(func() error {
				_, err := elasticsearch.DoRequest(ctx, esClient, http.MethodPut, "/"+pinnedIndex+"/_settings",
					[]byte(`{"index.routing.allocation.require._name": null}`))
				return err
			}),
		}).
		WithSteps(test.CheckTestSteps(mutated, k)).
		WithSteps(mutated.DeletionTestSteps(k)).
		RunSequential(t)
}

// pendingNodeSets returns a NodeSet with the given name and config whose Pod cannot be scheduled, and the same NodeSet
// once its Pod can be scheduled.
func pendingNodeSets(name string, config map[string]any) (pending esv1.NodeSet, fixed esv1.NodeSet) {
	pending = esv1.NodeSet{
		Name:        name,
		Count:       1,
		Config:      &commonv1.Config{Data: config},
		PodTemplate: elasticsearch.ESPodTemplate(elasticsearch.DefaultResources),
	}
	pending.PodTemplate.Spec.NodeSelector = map[string]string{"cannot": "be-scheduled"}
	fixed = *pending.DeepCopy()
	fixed.PodTemplate.Spec.NodeSelector = nil
	return pending, fixed
}

// createPinnedIndexStep returns a step that creates the pinned index with the given settings, which must prevent its
// shards from being migrated to the given nodes. It also initializes the given Elasticsearch client.
func createPinnedIndexStep(ctx context.Context, k *test.K8sClient, b elasticsearch.Builder, esClient *esclient.Client, nodes, settings string) test.Step {
	return test.Step{
		Name: "Create an index that cannot be migrated to " + nodes,
		Test: test.Eventually(func() error {
			var err error
			if *esClient, err = elasticsearch.NewElasticsearchClient(b.Elasticsearch, k); err != nil {
				return err
			}
			// the index may already exist if a previous attempt only failed to read the response
			if _, err := elasticsearch.DoRequest(ctx, *esClient, http.MethodGet, "/"+pinnedIndex, nil); err == nil {
				return nil
			}
			_, err = elasticsearch.DoRequest(ctx, *esClient, http.MethodPut, "/"+pinnedIndex, []byte(settings))
			return err
		}),
	}
}

// shutdownsKeptSteps returns the steps that check that the given leaving nodes get a remove-type shutdown, become not
// ready, and keep their shutdown over several reconciliations without being restarted.
func shutdownsKeptSteps(
	ctx context.Context,
	k *test.K8sClient,
	b elasticsearch.Builder,
	esClient *esclient.Client,
	leavingNodes []string,
	leavingPods *test.PodRestartChecker,
) test.StepList {
	esNamespace := b.Elasticsearch.Namespace
	var reconciledGeneration int64
	return test.StepList{
		{
			Name: "Leaving nodes should have a remove-type shutdown",
			Test: test.Eventually(func() error {
				return checkRemoveShutdowns(ctx, *esClient, leavingNodes)
			}),
		},
		{
			Name: "Leaving nodes should not be ready",
			Test: test.Eventually(func() error {
				return checkPodsNotReady(k, esNamespace, leavingNodes)
			}),
		},
		{
			// a spec change that does not affect the Pods, to make sure the operator reconciles while the leaving Pods
			// are not ready rather than the shutdowns being kept because it is not reconciling
			Name: "Change the StatefulSets revision history limit",
			Test: test.Eventually(func() error {
				var es esv1.Elasticsearch
				if err := k.Client.Get(ctx, k8s.ExtractNamespacedName(&b.Elasticsearch), &es); err != nil {
					return err
				}
				es.Spec.RevisionHistoryLimit = new(int32(5))
				if err := k.Client.Update(ctx, &es); err != nil {
					return err
				}
				reconciledGeneration = es.Generation
				return nil
			}),
		},
		{
			Name: "The operator should reconcile the change",
			Test: test.Eventually(func() error {
				var es esv1.Elasticsearch
				if err := k.Client.Get(ctx, k8s.ExtractNamespacedName(&b.Elasticsearch), &es); err != nil {
					return err
				}
				if es.Status.ObservedGeneration < reconciledGeneration {
					return fmt.Errorf("observed generation %d, expected %d", es.Status.ObservedGeneration, reconciledGeneration)
				}
				return nil
			}),
		},
		{
			// the shutdowns used to be cancelled by the next reconciliation once their Pods were not ready, then
			// requested again, so the state is sampled over several reconciliations
			Name: "Shutdowns of the leaving nodes should not be cancelled",
			Test: func(t *testing.T) {
				t.Helper()
				require.Never(t, func() bool {
					err := checkRemoveShutdowns(ctx, *esClient, leavingNodes)
					if err == nil {
						err = checkPodsNotReady(k, esNamespace, leavingNodes)
					}
					if err != nil {
						t.Log(err)
					}
					// errors while querying Elasticsearch or the API server are not considered as a cancellation
					return errors.Is(err, errNoRemoveShutdown) || errors.Is(err, errPodReady)
				}, shutdownsKeptFor, 2*time.Second, "a shutdown of the leaving nodes was cancelled")
			},
		},
		{
			Name: "Leaving Pods should not have been restarted",
			Test: func(t *testing.T) {
				t.Helper()
				require.NoError(t, leavingPods.CheckNotRestarted(ctx, k))
			},
		},
	}
}

var (
	// errNoRemoveShutdown is returned by checkRemoveShutdowns when a node has no remove-type shutdown.
	errNoRemoveShutdown = errors.New("no remove-type shutdown")
	// errPodReady is returned by checkPodsNotReady when a Pod is ready.
	errPodReady = errors.New("pod is ready")
)

// checkRemoveShutdowns returns an error wrapping errNoRemoveShutdown if one of the given nodes has no remove-type
// shutdown, or is not in the cluster anymore.
func checkRemoveShutdowns(ctx context.Context, esClient esclient.Client, nodeNames []string) error {
	_, err := removeShutdowns(ctx, esClient, nodeNames)
	return err
}

// checkRemoveShutdownsComplete returns an error if one of the given nodes has no complete remove-type shutdown.
func checkRemoveShutdownsComplete(ctx context.Context, esClient esclient.Client, nodeNames []string) error {
	shutdowns, err := removeShutdowns(ctx, esClient, nodeNames)
	if err != nil {
		return err
	}
	for name, s := range shutdowns {
		if s.Status != esclient.ShutdownComplete {
			return fmt.Errorf("shutdown of node %s is %s", name, s.Status)
		}
	}
	return nil
}

// removeShutdowns returns the remove-type shutdowns of the given nodes by node name. It returns an error wrapping
// errNoRemoveShutdown if one of the nodes has none, or is not in the cluster anymore.
func removeShutdowns(ctx context.Context, esClient esclient.Client, nodeNames []string) (map[string]esclient.NodeShutdown, error) {
	nodes, err := esClient.GetNodes(ctx)
	if err != nil {
		return nil, err
	}
	shutdowns, err := esClient.GetShutdown(ctx, nil)
	if err != nil {
		return nil, err
	}
	removing := map[string]esclient.NodeShutdown{}
	for _, s := range shutdowns.Nodes {
		if s.Is(esclient.Remove) {
			removing[s.NodeID] = s
		}
	}
	nameToID := map[string]string{}
	for id, node := range nodes.Nodes {
		nameToID[node.Name] = id
	}
	byName := make(map[string]esclient.NodeShutdown, len(nodeNames))
	for _, name := range nodeNames {
		id, inCluster := nameToID[name]
		if !inCluster {
			return nil, fmt.Errorf("node %s is not in the cluster: %w", name, errNoRemoveShutdown)
		}
		s, exists := removing[id]
		if !exists {
			return nil, fmt.Errorf("node %s: %w", name, errNoRemoveShutdown)
		}
		byName[name] = s
	}
	return byName, nil
}

// podListOptionsByName returns the list options to list the given StatefulSet Pods.
func podListOptionsByName(t *testing.T, namespace string, podNames ...string) []k8sclient.ListOption {
	t.Helper()
	requirement, err := labels.NewRequirement(appsv1.StatefulSetPodNameLabel, selection.In, podNames)
	require.NoError(t, err)
	return []k8sclient.ListOption{
		k8sclient.InNamespace(namespace),
		k8sclient.MatchingLabelsSelector{Selector: labels.NewSelector().Add(*requirement)},
	}
}

// checkPodsNotReady returns an error if one of the given Pods is ready.
func checkPodsNotReady(k *test.K8sClient, namespace string, podNames []string) error {
	for _, name := range podNames {
		pod, err := k.GetPod(namespace, name)
		if err != nil {
			return err
		}
		if k8s.IsPodReady(pod) {
			return fmt.Errorf("%s: %w", name, errPodReady)
		}
	}
	return nil
}
