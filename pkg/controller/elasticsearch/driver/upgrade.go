// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func (d *defaultDriver) handleRollingUpgrades(
	esClient esclient.Client,
	statefulSets sset.StatefulSetList,
) *reconciler.Results {
	results := &reconciler.Results{}

	// We need an up-to-date ES state, but avoid requesting information we may not need.
	esState := NewMemoizingESState(esClient)

	// Maybe upgrade some of the nodes.
	res := newRollingUpgrade(d, esClient, esState, statefulSets).run()
	results.WithResults(res)

	// Maybe re-enable shards allocation if upgraded nodes are back into the cluster.
	res = d.MaybeEnableShardsAllocation(esClient, esState, statefulSets)
	results.WithResults(res)

	return results
}

type rollingUpgradeCtx struct {
	client         k8s.Client
	ES             v1alpha1.Elasticsearch
	statefulSets   sset.StatefulSetList
	esClient       esclient.Client
	esState        ESState
	podUpgradeDone func(k8s.Client, ESState, types.NamespacedName, string) (bool, error)
	upgrader       func(statefulSet *appsv1.StatefulSet, newPartition int32) error
}

func newRollingUpgrade(
	d *defaultDriver,
	esClient esclient.Client,
	esState ESState,
	statefulSets sset.StatefulSetList,
) rollingUpgradeCtx {
	return rollingUpgradeCtx{
		client:         d.Client,
		ES:             d.ES,
		statefulSets:   statefulSets,
		esClient:       esClient,
		esState:        esState,
		podUpgradeDone: podUpgradeDone,
		upgrader:       d.upgradeStatefulSetPartition,
	}
}

func (ctx rollingUpgradeCtx) run() *reconciler.Results {
	results := &reconciler.Results{}

	// TODO: deal with multiple restarts at once, taking the changeBudget into account.
	//  We'd need to stop checking cluster health and do something smarter, since cluster health green check
	//  should be done **in between** restarts to make sense, which is pretty hard to do since we don't
	//  trigger restarts but just allow the sset controller to do it at its own pace.
	//  Instead of green health, we could look at shards status, taking into account nodes
	//  we scheduled for a restart (maybe not restarted yet).

	maxConcurrentUpgrades := 1
	scheduledUpgrades := 0

	// Only update 1 master node at a time, for safety and zen settings convenience.
	// This can slow down the upgrade, but the number of master nodes should be small anyway.
	maxMasterNodeUpgrades := 1
	scheduledMasterNodeUpgrades := 0

	for _, statefulSet := range ctx.statefulSets.ToUpdate() {
		// Inspect each pod, starting from the highest ordinal, and decrement the partition to allow
		// pod upgrades to go through, controlled by the StatefulSet controller.
		for partition := sset.GetPartition(statefulSet); partition >= 0; partition-- {
			if partition >= sset.GetReplicas(statefulSet) {
				continue
			}
			if scheduledUpgrades >= maxConcurrentUpgrades {
				return results.WithResult(defaultRequeue)
			}
			if label.IsMasterNodeSet(statefulSet) && scheduledMasterNodeUpgrades >= maxMasterNodeUpgrades {
				return results.WithResult(defaultRequeue)
			}

			// Do we need to upgrade that pod?
			podName := sset.PodName(statefulSet.Name, partition)
			podRef := types.NamespacedName{Namespace: statefulSet.Namespace, Name: podName}
			alreadyUpgraded, err := ctx.podUpgradeDone(ctx.client, ctx.esState, podRef, statefulSet.Status.UpdateRevision)
			if err != nil {
				return results.WithError(err)
			}
			if alreadyUpgraded {
				continue
			}

			// An upgrade is required for that pod.
			scheduledUpgrades++

			// Is the pod upgrade already scheduled?
			if partition == sset.GetPartition(statefulSet) {
				continue
			}

			// Is the cluster ready for the node upgrade?
			clusterReady, err := clusterReadyForNodeRestart(ctx.ES, ctx.esState)
			if err != nil {
				return results.WithError(err)
			}
			if !clusterReady {
				// retry later
				return results.WithResult(defaultRequeue)
			}

			log.Info("Preparing cluster for node restart", "namespace", ctx.ES.Namespace, "es_name", ctx.ES.Name)
			if err := prepareClusterForNodeRestart(ctx.esClient, ctx.esState); err != nil {
				return results.WithError(err)
			}

			if label.IsMasterNodeSet(statefulSet) {
				scheduledMasterNodeUpgrades++
				// TODO if the node is a master:
				//  - zen1: update minimum_master_node to account for master node deletion. Otherwise upgrading a 2-masters
				//   cluster provokes downtime since m_m_n=2.
				//   Problem: how to prevent this to be reverted at the next reconciliation, before the pod gets deleted?
				//  - zen2: set voting config exclusions: same problem, this is not easy. But since we only delete
				//   one master at a time, maybe it's not required?
			}

			// Upgrade the pod.
			if err := ctx.upgrader(&statefulSet, partition); err != nil {
				return results.WithError(err)
			}
		}
	}
	return results
}

func (d *defaultDriver) upgradeStatefulSetPartition(
	statefulSet *appsv1.StatefulSet,
	newPartition int32,
) error {
	// Node can be removed, update the StatefulSet rollingUpdate.Partition ordinal.
	log.Info("Updating rollingUpdate.Partition",
		"namespace", statefulSet.Namespace,
		"name", statefulSet.Name,
		"from", statefulSet.Spec.UpdateStrategy.RollingUpdate.Partition,
		"to", &newPartition,
	)

	nodespec.UpdatePartition(statefulSet, &newPartition)
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
