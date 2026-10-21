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
	corev1 "k8s.io/api/core/v1"
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
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/shutdown"
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
	// pendingObservedFor is how long a Pod that cannot be scheduled is observed for, for the operator to reconcile it
	pendingObservedFor = 30 * time.Second
)

// TestDownscaleKeepsShutdownsWhileAnotherPodIsNotReady removes a NodeSet while the Pod of another NodeSet cannot be
// scheduled. The readiness probe makes the leaving nodes not ready as soon as their remove-type shutdown is registered,
// which must not make the downscale use up its budget and cancel the shutdowns it is waiting for.
func TestDownscaleKeepsShutdownsWhileAnotherPodIsNotReady(t *testing.T) {
	skipBeforeReadinessPort(t)

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
		WithStep(createPinnedIndexStep(ctx, k, initial, &esClient, pinnedIndex, "the new data nodes", pinnedIndexSettings)).
		WithStep(elasticsearch.CheckClusterHealth(initial, k)).
		WithSteps(leavingPods.RecordUIDs(k)).
		WithSteps(renamedWithPending.UpgradeTestSteps(k)).
		WithSteps(shutdownsKeptSteps(ctx, k, initial, &esClient, leavingNodes, leavingPods)).
		WithStep(unpinIndexStep(ctx, &esClient, pinnedIndex, "the new data nodes", "exclude._name")).
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
	skipBeforeReadinessPort(t)

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
		WithStep(createPinnedIndexStep(ctx, k, initial, &esClient, pinnedIndex, "the remaining master nodes", pinnedIndexSettings)).
		WithStep(elasticsearch.CheckClusterHealth(initial, k)).
		WithSteps(masterPods.RecordUIDs(k)).
		WithSteps(downscaledWithPending.UpgradeTestSteps(k)).
		WithSteps(shutdownsKeptSteps(ctx, k, initial, &esClient, []string{leavingNode}, masterPods)).
		WithStep(unpinIndexStep(ctx, &esClient, pinnedIndex, "the remaining master nodes", "require._name")).
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
	skipBeforeReadinessPort(t)

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
		WithStep(createPinnedIndexStep(ctx, k, initial, &esClient, pinnedIndex, "the remaining master nodes", pinnedIndexSettings)).
		WithStep(elasticsearch.CheckClusterHealth(initial, k)).
		WithSteps(masterPods.RecordUIDs(k)).
		WithSteps(downscaledWithPending.UpgradeTestSteps(k)).
		WithSteps(shutdownsKeptSteps(ctx, k, initial, &esClient, []string{leavingNode}, masterPods)).
		WithStep(unpinIndexStep(ctx, &esClient, pinnedIndex, "the remaining master nodes", "require._name")).
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

// TestDownscaleDuringRollingUpgrade removes data nodes, then applies a pod template change to all the NodeSets once the
// leaving nodes are draining and not ready. The leaving nodes are not upgraded: their shutdowns are kept and their Pods
// are not restarted. The staying data node is only upgraded once they are removed: the tier availability predicate counts
// the not ready leaving nodes as data nodes, so it holds the restart of the only healthy one. Applying the change once the
// leaving nodes are not ready makes this order deterministic, rather than depending on whether the operator selects the
// staying node before the readiness probes fail.
func TestDownscaleDuringRollingUpgrade(t *testing.T) {
	skipBeforeReadinessPort(t)

	ctx := t.Context()
	k := test.NewK8sClientOrFatal()

	initial := elasticsearch.NewBuilder("test-downscale-rolling-upgrade").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(3, "data", elasticsearch.DefaultResources)
	// the not ready leaving nodes must not prevent the staying nodes from being upgraded
	initial.Elasticsearch.Spec.UpdateStrategy.ChangeBudget.MaxUnavailable = new(int32(3))

	downscaled := initial.WithNoESTopology().
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(1, "data", elasticsearch.DefaultResources)
	// built separately from downscaled, as WithEnvironmentVariable updates the NodeSets it shares with it
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
		WithStep(createPinnedIndexStep(ctx, k, initial, &esClient, pinnedIndex, "the staying data node", pinnedIndexSettings)).
		WithStep(elasticsearch.CheckClusterHealth(initial, k)).
		WithSteps(leavingPods.RecordUIDs(k)).
		WithSteps(stayingDataPod.RecordUIDs(k)).
		WithSteps(masterPod.RecordUIDs(k)).
		WithSteps(downscaled.UpgradeTestSteps(k)).
		WithSteps(shutdownsKeptSteps(ctx, k, initial, &esClient, leavingNodes, leavingPods)).
		// the leaving nodes are not ready: the pod template change makes them rolling upgrade candidates while they drain
		WithSteps(mutated.UpgradeTestSteps(k)).
		// the operator reconciles the pod template change while the leaving nodes drain, which must not restart them nor
		// cancel their shutdowns
		WithSteps(shutdownsKeptSteps(ctx, k, mutated, &esClient, leavingNodes, leavingPods)).
		WithStep(unpinIndexStep(ctx, &esClient, pinnedIndex, "the staying data node", "require._name")).
		// once the leaving nodes are removed, the staying data node is the only one of its tier and can be upgraded
		WithSteps(stayingDataPod.WaitForRestart(k)).
		WithSteps(masterPod.WaitForRestart(k)).
		WithSteps(test.CheckTestSteps(mutated, k)).
		WithSteps(mutated.DeletionTestSteps(k)).
		RunSequential(t)
}

// TestDownscaleRecreatesLeavingPodThatNeverJoined reduces a NodeSet while the Pod of its last node never joined the
// cluster, because it cannot be scheduled, and repairs its spec with the same change. The other nodes of the NodeSet are
// healthy, so the forced upgrade does not apply. No shutdown can be requested for a node that is not a member of the
// cluster: the downscale must hold it, and not fail, for the rolling upgrade to recreate the Pod with the repaired spec.
// It is then removed once it has joined the cluster, without restarting the healthy nodes.
func TestDownscaleRecreatesLeavingPodThatNeverJoined(t *testing.T) {
	// the shutdown API is not used before this version, a node that holds no shard is removed whether it is a member or not
	if version.MustParse(test.Ctx().ElasticStackVersion).LT(shutdown.MinVersion) {
		t.SkipNow()
	}

	ctx := t.Context()
	k := test.NewK8sClientOrFatal()

	initial := elasticsearch.NewBuilder("test-downscale-never-joined").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(2, "data", elasticsearch.DefaultResources)

	// add a data node whose Pod cannot be scheduled: the template of the NodeSet changes for the nodes that are healthy,
	// which are only restarted for it once the new Pod is ready, so they are not
	pending := initial.WithNoESTopology().
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(3, "data", elasticsearch.DefaultResources)
	for i := range pending.Elasticsearch.Spec.NodeSets {
		if pending.Elasticsearch.Spec.NodeSets[i].Name == "data" {
			pending.Elasticsearch.Spec.NodeSets[i].PodTemplate.Spec.NodeSelector = map[string]string{"cannot": "be-scheduled"}
		}
	}

	esName := initial.Elasticsearch.Name
	esNamespace := initial.Elasticsearch.Namespace
	dataStatefulSet := esv1.StatefulSet(esName, "data")
	healthyNodes := []string{sset.PodName(dataStatefulSet, 0), sset.PodName(dataStatefulSet, 1)}
	pendingNode := sset.PodName(dataStatefulSet, 2)
	healthyPods := test.NewPodRestartChecker("healthy data Elasticsearch", podListOptionsByName(t, esNamespace, healthyNodes...)...)
	var pendingUID types.UID

	test.StepList{}.
		WithSteps(initial.InitTestSteps(k)).
		WithSteps(initial.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(initial, k)).
		WithSteps(healthyPods.RecordUIDs(k)).
		WithSteps(pending.UpgradeTestSteps(k)).
		WithStep(test.Step{
			Name: "The new data Pod should be pending while the other ones are ready",
			Test: test.Eventually(func() error {
				pod, err := k.GetPod(esNamespace, pendingNode)
				if err != nil {
					return err
				}
				if pod.Status.Phase != corev1.PodPending {
					return fmt.Errorf("pod %s is %s, expected %s", pendingNode, pod.Status.Phase, corev1.PodPending)
				}
				for _, name := range healthyNodes {
					healthyPod, err := k.GetPod(esNamespace, name)
					if err != nil {
						return err
					}
					if !k8s.IsPodReady(healthyPod) {
						return fmt.Errorf("pod %s is not ready", name)
					}
				}
				pendingUID = pod.UID
				return nil
			}),
		}).
		WithStep(test.Step{
			Name: "The healthy data Pods should not be restarted while the new one is pending",
			Test: func(t *testing.T) {
				t.Helper()
				require.Never(t, func() bool {
					err := healthyPods.CheckNotRestarted(ctx, k)
					if err != nil {
						t.Log(err)
					}
					return err != nil
				}, pendingObservedFor, 2*time.Second, "a healthy data Pod was restarted")
			},
		}).
		// repair the spec while reducing the NodeSet: the Pod that is pending is the one to remove
		WithSteps(initial.UpgradeTestSteps(k)).
		WithStep(test.Step{
			Name: "The Pod that never joined the cluster should be recreated",
			Test: test.Eventually(func() error {
				pod, err := k.GetPod(esNamespace, pendingNode)
				if err != nil {
					return err
				}
				if pod.UID == pendingUID {
					return fmt.Errorf("pod %s has not been recreated", pendingNode)
				}
				return nil
			}),
		}).
		// the recreated Pod joins the cluster, and is then removed
		WithSteps(test.CheckTestSteps(initial, k)).
		WithStep(test.Step{
			Name: "The healthy data Pods should not have been restarted",
			Test: func(t *testing.T) {
				t.Helper()
				require.NoError(t, healthyPods.CheckNotRestarted(ctx, k))
			},
		}).
		WithSteps(initial.DeletionTestSteps(k)).
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

// createPinnedIndexStep returns a step that creates the given index with the given settings, which must prevent its
// shards from being migrated to the given nodes. It also initializes the given Elasticsearch client.
func createPinnedIndexStep(ctx context.Context, k *test.K8sClient, b elasticsearch.Builder, esClient *esclient.Client, index, nodes, settings string) test.Step {
	return test.Step{
		Name: "Create an index that cannot be migrated to " + nodes,
		Test: test.Eventually(func() error {
			var err error
			if *esClient, err = elasticsearch.NewElasticsearchClient(b.Elasticsearch, k); err != nil {
				return err
			}
			// the index may already exist if a previous attempt only failed to read the response
			if _, err := elasticsearch.DoRequest(ctx, *esClient, http.MethodGet, "/"+index, nil); err == nil {
				return nil
			}
			_, err = elasticsearch.DoRequest(ctx, *esClient, http.MethodPut, "/"+index, []byte(settings))
			return err
		}),
	}
}

// unpinIndexStep returns a step that clears the given allocation setting of the given index, so that its shards can be
// migrated to the given nodes.
func unpinIndexStep(ctx context.Context, esClient *esclient.Client, index, nodes, setting string) test.Step {
	return test.Step{
		Name: "Allow the index to be migrated to " + nodes,
		Test: test.Eventually(func() error {
			_, err := elasticsearch.DoRequest(ctx, *esClient, http.MethodPut, "/"+index+"/_settings",
				[]byte(`{"index.routing.allocation.`+setting+`": null}`))
			return err
		}),
	}
}

// skipBeforeReadinessPort skips the test before the readiness port is used: nodes with a registered shutdown are only
// reported not ready by the readiness port, used after 8.2.0.
func skipBeforeReadinessPort(t *testing.T) {
	t.Helper()
	if version.MustParse(test.Ctx().ElasticStackVersion).LE(esv1.MinReadinessPortVersion) {
		t.SkipNow()
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
			// a spec change that does not affect the StatefulSets nor the Pods, to make sure the operator reconciles while
			// the leaving Pods are not ready rather than the shutdowns being kept because it is not reconciling
			Name: "Change the HTTP Service annotations",
			Test: test.Eventually(func() error {
				var es esv1.Elasticsearch
				if err := k.Client.Get(ctx, k8s.ExtractNamespacedName(&b.Elasticsearch), &es); err != nil {
					return err
				}
				es.Spec.HTTP.Service.ObjectMeta.Annotations = map[string]string{"e2e-downscale-reconcile": "true"}
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
