// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"context"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (d *defaultDriver) handleRollingUpgrades(
	esClient esclient.Client,
	esState ESState,
	statefulSets sset.StatefulSetList,
	expectedMaster []string,
) *reconciler.Results {
	results := &reconciler.Results{}

	// We need to check that all the expectations are met before continuing.
	// This is to be sure that none of the previous steps has changed the state and
	// that we are not running with a stale cache.
	ok, err := d.expectationsMet(statefulSets)
	if err != nil {
		return results.WithError(err)
	}
	if !ok {
		return results.WithResult(defaultRequeue)
	}

	// Get the healthy Pods (from a K8S point of view + in the ES cluster)
	healthyPods, err := healthyPods(d.Client, statefulSets, esState)
	if err != nil {
		return results.WithError(err)
	}

	// Get the pods to upgrade
	podsToUpgrade, err := podsToUpgrade(d.Client, statefulSets)
	if err != nil {
		return results.WithError(err)
	}

	// Maybe upgrade some of the nodes.
	deletedPods, err := newRollingUpgrade(
		d,
		esClient,
		esState,
		statefulSets,
		d.Expectations,
		expectedMaster,
		podsToUpgrade,
		healthyPods,
	).run()
	if err != nil {
		return results.WithError(err)
	}
	if len(deletedPods) > 0 {
		// Some Pods have just been deleted, we don't need to try to enable shards allocation.
		return results.WithResult(defaultRequeue)
	}
	if len(podsToUpgrade) > len(deletedPods) {
		// Some Pods have not been update, ensure that we retry later
		results.WithResult(defaultRequeue)
	}

	// Maybe re-enable shards allocation if upgraded nodes are back into the cluster.
	res := d.MaybeEnableShardsAllocation(esClient, esState, statefulSets)
	results.WithResults(res)

	return results
}

type rollingUpgradeCtx struct {
	client          k8s.Client
	ES              v1alpha1.Elasticsearch
	statefulSets    sset.StatefulSetList
	esClient        esclient.Client
	esState         ESState
	expectations    *expectations.Expectations
	expectedMasters []string
	podUpgradeDone  func(pod corev1.Pod, expectedRevision string) (bool, error)
	podsToUpgrade   []corev1.Pod
	healthyPods     map[string]corev1.Pod
}

func newRollingUpgrade(
	d *defaultDriver,
	esClient esclient.Client,
	esState ESState,
	statefulSets sset.StatefulSetList,
	expectations *expectations.Expectations,
	expectedMaster []string,
	podsToUpgrade []corev1.Pod,
	healthyPods map[string]corev1.Pod,
) rollingUpgradeCtx {
	return rollingUpgradeCtx{
		client:          d.Client,
		ES:              d.ES,
		statefulSets:    statefulSets,
		esClient:        esClient,
		esState:         esState,
		expectations:    expectations,
		podUpgradeDone:  podUpgradeDone,
		expectedMasters: expectedMaster,
		podsToUpgrade:   podsToUpgrade,
		healthyPods:     healthyPods,
	}
}

func (ctx rollingUpgradeCtx) run() ([]corev1.Pod, error) {
	deletionDriver := NewDeletionDriver(
		ctx.client,
		ctx.esClient,
		&ctx.ES,
		ctx.statefulSets,
		ctx.esState,
		ctx.expectedMasters,
		ctx.healthyPods,
		ctx.podsToUpgrade,
		ctx.expectations,
	)
	deletedPods, err := deletionDriver.Delete(ctx.podsToUpgrade)
	if errors.IsConflict(err) || errors.IsNotFound(err) {
		// Cache is not up to date or Pod has been deleted by someone else
		// (could be the statefulset controller)
		// TODO: should we at least log this one in debug mode ?
		return deletedPods, nil
	}
	if err != nil {
		return deletedPods, err
	}
	return deletedPods, nil
}

func healthyPods(
	client k8s.Client,
	statefulSets sset.StatefulSetList,
	esState ESState,
) (map[string]corev1.Pod, error) {
	healthyPods := make(map[string]corev1.Pod)
	currentPods, err := statefulSets.GetActualPods(client)
	if err != nil {
		return nil, err
	}
	for _, pod := range currentPods {
		if !pod.DeletionTimestamp.IsZero() || !k8s.IsPodReady(pod) {
			continue
		}
		// has the node joined the cluster yet?
		inCluster, err := esState.NodesInCluster([]string{pod.Name})
		if err != nil {
			return nil, err
		}
		if inCluster {
			healthyPods[pod.Name] = pod
		}
	}
	return healthyPods, nil
}

func podsToUpgrade(
	client k8s.Client,
	statefulSets sset.StatefulSetList,
) ([]corev1.Pod, error) {
	var toBeDeletedPods []corev1.Pod
	toUpdate := statefulSets.ToUpdate()
	for _, statefulSet := range toUpdate {
		// Inspect each pod, starting from the highest ordinal, and decrement the idx to allow
		// pod upgrades to go through, controlled by the StatefulSet controller.
		for idx := sset.GetReplicas(statefulSet) - 1; idx >= 0; idx-- {
			// Do we need to upgrade that pod?
			podName := sset.PodName(statefulSet.Name, idx)
			podRef := types.NamespacedName{Namespace: statefulSet.Namespace, Name: podName}
			// retrieve pod to inspect its revision label
			var pod corev1.Pod
			err := client.Get(podRef, &pod)
			if err != nil && !errors.IsNotFound(err) {
				return toBeDeletedPods, err
			}
			if errors.IsNotFound(err) {
				// Pod does not exist, continue the loop as the absence will be accounted by the deletion driver
				continue
			}
			alreadyUpgraded, err := podUpgradeDone(pod, statefulSet.Status.UpdateRevision)
			if err != nil {
				return toBeDeletedPods, err
			}
			if !alreadyUpgraded {
				toBeDeletedPods = append(toBeDeletedPods, pod)
			}
		}
	}
	return toBeDeletedPods, nil
}

func prepareClusterForNodeRestart(esClient esclient.Client, esState ESState) error {
	// Disable shard allocations to avoid shards moving around while the node is temporarily down
	shardsAllocationEnabled, err := esState.ShardAllocationsEnabled()
	if err != nil {
		return err
	}
	if shardsAllocationEnabled {
		if err := disableShardsAllocation(esClient); err != nil {
			return err
		}
	}

	// Request a sync flush to optimize indices recovery when the node restarts.
	if err := doSyncFlush(esClient); err != nil {
		return err
	}

	// TODO: halt ML jobs on that node
	return nil
}

// clusterReadyForNodeRestart returns true if the ES cluster allows a node to be restarted
// with minimized downtime and no unexpected data loss.
func clusterReadyForNodeRestart(es v1alpha1.Elasticsearch, esState ESState) (bool, error) {
	// Check the cluster health: only allow node restart if health is green.
	// This would cause downtime if some shards have 0 replicas, but we consider that's on the user.
	// TODO: we could technically still restart a node if the cluster is yellow,
	//  as long as there are other copies of the shards in-sync on other nodes
	// TODO: the fact we rely on a cached health here would prevent more than 1 restart
	//  in a single reconciliation
	green, err := esState.GreenHealth()
	if err != nil {
		return false, err
	}
	if !green {
		log.Info("Skipping node rolling upgrade since cluster is not green", "namespace", es.Namespace, "name", es.Name)
		return false, nil
	}
	return true, nil
}

// podUpgradeDone inspects the given pod and returns true if it was successfully upgraded.
func podUpgradeDone(pod corev1.Pod, expectedRevision string) (bool, error) {
	if expectedRevision == "" {
		// no upgrade scheduled for the sset
		return false, nil
	}
	if sset.PodRevision(pod) != expectedRevision {
		// pod revision does not match the sset upgrade revision
		return false, nil
	}
	return true, nil
}

func disableShardsAllocation(esClient esclient.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	return esClient.DisableReplicaShardsAllocation(ctx)
}

func doSyncFlush(esClient esclient.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	return esClient.SyncedFlush(ctx)
}

func (d *defaultDriver) MaybeEnableShardsAllocation(
	esClient esclient.Client,
	esState ESState,
	statefulSets sset.StatefulSetList,
) *reconciler.Results {
	results := &reconciler.Results{}
	alreadyEnabled, err := esState.ShardAllocationsEnabled()
	if err != nil {
		return results.WithError(err)
	}
	if alreadyEnabled {
		return results
	}

	// Make sure all pods scheduled for upgrade have been upgraded.
	done, err := statefulSets.PodReconciliationDone(d.Client)
	if err != nil {
		return results.WithError(err)
	}
	if !done {
		log.V(1).Info(
			"Rolling upgrade not over yet, some pods don't have the updated revision, keeping shard allocations disabled",
			"namespace", d.ES.Namespace,
			"es_name", d.ES.Name,
		)
		return results.WithResult(defaultRequeue)
	}

	// Check if we have some deletions in progress
	satisfiedDeletion, err := d.Expectations.SatisfiedDeletions(k8s.ExtractNamespacedName(&d.ES), d)
	if err != nil {
		return results.WithError(err)
	}
	if !satisfiedDeletion {
		log.V(1).Info(
			"Rolling upgrade not over yet, still waiting for some Pods to be deleted, keeping shard allocations disabled",
			"namespace", d.ES.Namespace,
			"es_name", d.ES.Name,
		)
		return results.WithResult(defaultRequeue)
	}

	// Make sure all nodes scheduled for upgrade are back into the cluster.
	nodesInCluster, err := esState.NodesInCluster(statefulSets.PodNames())
	if err != nil {
		return results.WithError(err)
	}
	if !nodesInCluster {
		log.V(1).Info(
			"Some upgraded nodes are not back in the cluster yet, keeping shard allocations disabled",
			"namespace", d.ES.Namespace,
			"es_name", d.ES.Name,
		)
		return results.WithResult(defaultRequeue)
	}

	log.Info("Enabling shards allocation", "namespace", d.ES.Namespace, "es_name", d.ES.Name)
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	if err := esClient.EnableShardAllocation(ctx); err != nil {
		return results.WithError(err)
	}

	return results
}
