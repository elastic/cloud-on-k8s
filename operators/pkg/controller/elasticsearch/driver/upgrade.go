// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"context"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (d *defaultDriver) handleRollingUpgrades(
	es v1alpha1.Elasticsearch,
	esClient esclient.Client,
	statefulSets sset.StatefulSetList,
) *reconciler.Results {
	results := &reconciler.Results{}

	// We need an up-to-date ES state, but avoid requesting information we may not need.
	esState := NewLazyESState(esClient)

	// Maybe upgrade some of the nodes.
	res := d.doRollingUpgrade(es, statefulSets, esClient, esState)
	results.WithResults(res)

	// Maybe re-enable shards allocation if upgraded nodes are back into the cluster.
	res = d.MaybeEnableShardsAllocation(es, esClient, esState, statefulSets)
	results.WithResults(res)

	return results
}

func (d *defaultDriver) doRollingUpgrade(
	es v1alpha1.Elasticsearch,
	statefulSets sset.StatefulSetList,
	esClient esclient.Client,
	esState ESState,
) *reconciler.Results {
	results := &reconciler.Results{}

	if !d.expectations.GenerationExpected(statefulSets.ObjectMetas()...) {
		// Our cache of SatefulSets is out of date compared to previous reconciliation operations.
		// It does not matter much here since operations are idempotent, but we might as well avoid
		// useless operations that would end up in a resource update conflict anyway.
		log.V(1).Info("StatefulSet cache out-of-date, re-queueing")
		return results.WithResult(defaultRequeue)
	}

	if !statefulSets.RevisionUpdateScheduled() {
		// nothing to upgrade
		return results
	}

	// TODO: deal with multiple restarts at once, taking the changeBudget into account.
	//  We'd need to stop checking cluster health and do something smarter, since cluster health green check
	//  should be done **in between** restarts to make sense, which is pretty hard to do since we don't
	//  trigger restarts but just allow the sset controller to do it at its own pace.
	//  Instead of green health, we could look at shards status, taking into account nodes
	//  we scheduled for a restart (maybe not restarted yet).

	// TODO: don't upgrade more than 1 master concurrently (ok for now since we upgrade 1 node at a time anyway)

	maxConcurrentUpgrades := 1
	scheduledUpgrades := 0

	for i, statefulSet := range statefulSets {
		// Inspect each pod, starting from the highest ordinal, and decrement the partition to allow
		// pod upgrades to go through, controlled by the StatefulSet controller.
		for partition := sset.GetUpdatePartition(statefulSet); partition >= 0; partition-- {
			if scheduledUpgrades >= maxConcurrentUpgrades {
				return results.WithResult(defaultRequeue)
			}
			if partition >= sset.Replicas(statefulSet) {
				continue
			}

			// Do we need to upgrade that pod?
			podName := sset.PodName(statefulSet.Name, partition)
			podRef := types.NamespacedName{Namespace: statefulSet.Namespace, Name: podName}
			alreadyUpgraded, err := podUpgradeDone(d.Client, esState, podRef, statefulSet.Status.UpdateRevision)
			if err != nil {
				return results.WithError(err)
			}
			if alreadyUpgraded {
				continue
			}

			// An upgrade is required for that pod.
			scheduledUpgrades++

			// Is the pod upgrade already scheduled?
			if partition == sset.GetUpdatePartition(statefulSet) {
				continue
			}

			// Is the cluster ready for the node upgrade?
			clusterReady, err := clusterReadyForNodeRestart(es, esState)
			if err != nil {
				return results.WithError(err)
			}
			if !clusterReady {
				// retry later
				return results.WithResult(defaultRequeue)
			}

			log.Info("Preparing cluster for node restart", "namespace", es.Namespace, "name", es.Name)
			if err := prepareClusterForNodeRestart(esClient, esState); err != nil {
				return results.WithError(err)
			}

			// Upgrade the pod.
			if err := d.upgradeStatefulSetPartition(&statefulSets[i], partition); err != nil {
				return results.WithError(err)
			}
			scheduledUpgrades++
		}
	}
	return results
}

func (d *defaultDriver) upgradeStatefulSetPartition(
	statefulSet *appsv1.StatefulSet,
	newPartition int32,
) error {
	// TODO: zen1, zen2

	// Node can be removed, update the StatefulSet rollingUpdate.Partition ordinal.
	log.Info("Updating rollingUpdate.Partition",
		"namespace", statefulSet.Namespace,
		"name", statefulSet.Name,
		"from", statefulSet.Spec.UpdateStrategy.RollingUpdate.Partition,
		"to", &newPartition,
	)
	statefulSet.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{
		Partition: &newPartition,
	}
	if err := d.Client.Update(statefulSet); err != nil {
		return err
	}

	// Register the updated sset generation to deal with out-of-date sset cache.
	d.Expectations.ExpectGeneration(statefulSet.ObjectMeta)

	return nil
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
func podUpgradeDone(c k8s.Client, esState ESState, podRef types.NamespacedName, expectedRevision string) (bool, error) {
	if expectedRevision == "" {
		// no upgrade scheduled for the sset
		return false, nil
	}
	// retrieve pod to inspect its revision label
	var pod corev1.Pod
	err := c.Get(podRef, &pod)
	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}
	if errors.IsNotFound(err) || !pod.DeletionTimestamp.IsZero() {
		// pod is terminating
		return false, nil
	}
	if sset.PodRevision(pod) != expectedRevision {
		// pod revision does not match the sset upgrade revision
		return false, nil
	}
	// is the pod ready?
	if !k8s.IsPodReady(pod) {
		return false, nil
	}
	// has the node joined the cluster yet?
	inCluster, err := esState.NodesInCluster([]string{podRef.Name})
	if err != nil {
		return false, err
	}
	if !inCluster {
		log.V(1).Info("Node has not joined the cluster yet", "namespace", podRef.Namespace, "name", podRef.Name)
		return false, err
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
	es v1alpha1.Elasticsearch,
	esClient esclient.Client,
	esState ESState,
	statefulSets sset.StatefulSetList,
) *reconciler.Results {
	results := &reconciler.Results{}
	// Since we rely on sset rollingUpdate.Partition, requeue in case our cache hasn't seen a sset update yet.
	// Otherwise we could re-enable shards allocation while a pod was just scheduled for termination,
	// with the partition in the sset cache being outdated.
	if !d.Expectations.GenerationExpected(statefulSets.ObjectMetas()...) {
		return results.WithResult(defaultRequeue)
	}

	alreadyEnabled, err := esState.ShardAllocationsEnabled()
	if err != nil {
		return results.WithError(err)
	}
	if alreadyEnabled {
		return results
	}

	// Make sure all pods scheduled for upgrade have been upgraded.
	scheduledUpgradesDone, err := sset.ScheduledUpgradesDone(d.Client, statefulSets)
	if err != nil {
		return results.WithError(err)
	}
	if !scheduledUpgradesDone {
		log.V(1).Info(
			"Rolling upgrade not over yet, some pods don't have the updated revision, keeping shard allocations disabled",
			"namespace", es.Namespace,
			"name", es.Name,
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
			"namespace", es.Namespace,
			"name", es.Name,
		)
		return results.WithResult(defaultRequeue)
	}

	log.Info("Enabling shards allocation", "namespace", es.Namespace, "name", es.Name)
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	if err := esClient.EnableShardAllocation(ctx); err != nil {
		return results.WithError(err)
	}

	return results
}
