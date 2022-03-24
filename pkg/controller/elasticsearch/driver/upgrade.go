// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/shutdown"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func (d *defaultDriver) handleUpgrades(
	ctx context.Context,
	esClient esclient.Client,
	esState ESState,
	expectedResources nodespec.ResourcesList,
) *reconciler.Results {
	results := &reconciler.Results{}

	// We need to check that all the expectations are satisfied before continuing.
	// This is to be sure that none of the previous steps has changed the state and
	// that we are not running with a stale cache.
	ok, reason, err := d.expectationsSatisfied()
	if err != nil {
		return results.WithError(err)
	}
	if !ok {
		reason := fmt.Sprintf("Nodes upgrade: %s", reason)
		return results.WithReconciliationState(defaultRequeue.WithReason(reason))
	}

	// Get the pods to upgrade
	statefulSets, err := sset.RetrieveActualStatefulSets(d.Client, k8s.ExtractNamespacedName(&d.ES))
	if err != nil {
		return results.WithError(err)
	}
	podsToUpgrade, err := podsToUpgrade(d.Client, statefulSets)
	if err != nil {
		return results.WithError(err)
	}
	// Get the healthy Pods (from a K8S point of view + in the ES cluster)
	healthyPods, err := healthyPods(d.Client, statefulSets, esState)
	if err != nil {
		return results.WithError(err)
	}

	nodeNameToID, err := esState.NodeNameToID()
	if err != nil {
		results.WithError(err)
	}
	logger := log.WithValues("namespace", d.ES.Namespace, "es_name", d.ES.Name)
	nodeShutdown := shutdown.NewNodeShutdown(esClient, nodeNameToID, esclient.Restart, d.ES.ResourceVersion, logger)

	// Get the list of pods currently existing in the StatefulSetList
	currentPods, err := statefulSets.GetActualPods(d.Client)
	if err != nil {
		return results.WithError(err)
	}

	expectedMasters := expectedResources.MasterNodesNames()

	// Maybe upgrade some of the nodes.
	upgrade := newUpgrade(
		ctx,
		d,
		statefulSets,
		expectedResources,
		esClient,
		esState,
		nodeShutdown,
		expectedMasters,
		podsToUpgrade,
		healthyPods,
		currentPods,
	)

	var deletedPods []corev1.Pod

	isVersionUpgrade, err := isVersionUpgrade(d.ES)
	if err != nil {
		return results.WithError(err)
	}
	shouldDoFullRestartUpgrade := isNonHACluster(currentPods, expectedMasters) && isVersionUpgrade
	if shouldDoFullRestartUpgrade {
		// unconditional full cluster upgrade
		deletedPods, err = run(upgrade.DeleteAll)
	} else {
		// regular rolling upgrade
		deletedPods, err = run(upgrade.Delete)
	}
	if err != nil {
		return results.WithError(err)
	}
	if len(deletedPods) > 0 {
		// Some Pods have just been deleted, we don't need to try to enable shards allocation.
		return results.WithReconciliationState(defaultRequeue.WithReason("Nodes upgrade in progress"))
	}
	if len(podsToUpgrade) > len(deletedPods) {
		// Some Pods have not been updated, ensure that we retry later
		results.WithReconciliationState(defaultRequeue.WithReason("Nodes upgrade in progress"))
	}

	// Maybe re-enable shards allocation and delete shutdowns if upgraded nodes are back into the cluster.
	res := d.maybeCompleteNodeUpgrades(ctx, esClient, esState, nodeShutdown)
	results.WithResults(res)

	return results
}

type upgradeCtx struct {
	parentCtx       context.Context
	client          k8s.Client
	ES              esv1.Elasticsearch
	resourcesList   nodespec.ResourcesList
	statefulSets    sset.StatefulSetList
	esClient        esclient.Client
	shardLister     esclient.ShardLister
	nodeShutdown    *shutdown.NodeShutdown
	esState         ESState
	expectations    *expectations.Expectations
	reconcileState  *reconcile.State
	expectedMasters []string
	podsToUpgrade   []corev1.Pod
	healthyPods     map[string]corev1.Pod
	currentPods     []corev1.Pod
}

func newUpgrade(
	ctx context.Context,
	d *defaultDriver,
	statefulSets sset.StatefulSetList,
	resourcesList nodespec.ResourcesList,
	esClient esclient.Client,
	esState ESState,
	nodeShutdown *shutdown.NodeShutdown,
	expectedMaster []string,
	podsToUpgrade []corev1.Pod,
	healthyPods map[string]corev1.Pod,
	currentPods []corev1.Pod,
) upgradeCtx {
	return upgradeCtx{
		parentCtx:       ctx,
		client:          d.Client,
		ES:              d.ES,
		statefulSets:    statefulSets,
		resourcesList:   resourcesList,
		esClient:        esClient,
		shardLister:     esClient,
		nodeShutdown:    nodeShutdown,
		esState:         esState,
		expectations:    d.Expectations,
		reconcileState:  d.ReconcileState,
		expectedMasters: expectedMaster,
		podsToUpgrade:   podsToUpgrade,
		healthyPods:     healthyPods,
		currentPods:     currentPods,
	}
}

func run(upgrade func() ([]corev1.Pod, error)) ([]corev1.Pod, error) {
	deletedPods, err := upgrade()
	if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
		// Cache is not up to date or Pod has been deleted by someone else
		// (could be the StatefulSet controller)
		// TODO: should we at least log this one in debug mode ?
		return deletedPods, nil
	}
	if err != nil {
		return deletedPods, err
	}
	return deletedPods, nil
}

// isNonHACluster returns true if the expected and actual number of master nodes indicates that the quorum of that cluster
// does not allow the loss of any node in which case a regular rolling upgrade might not be possible especially when doing
// a major version upgrade.
func isNonHACluster(actualPods []corev1.Pod, expectedMasters []string) bool {
	if len(expectedMasters) > 2 {
		return false
	}
	actualMasters := label.FilterMasterNodePods(actualPods)
	return len(actualMasters) <= 2
}

// isVersionUpgrade returns true if a spec change contains a version upgrade.
func isVersionUpgrade(es esv1.Elasticsearch) (bool, error) {
	specVersion, err := version.Parse(es.Spec.Version)
	if err != nil {
		return false, err
	}
	statusVersion, err := version.Parse(es.Status.Version)
	if err != nil {
		return false, err
	}
	return specVersion.GT(statusVersion), nil
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

// podsToUpgrade returns all Pods of all StatefulSets where the controller-revision-hash label compared to the sset's
// .status.updateRevision indicates that the Pod still needs to be deleted to be recreated with the new spec.
func podsToUpgrade(
	client k8s.Client,
	statefulSets sset.StatefulSetList,
) ([]corev1.Pod, error) {
	var toUpgrade []corev1.Pod
	for _, statefulSet := range statefulSets {
		if statefulSet.Status.UpdateRevision == "" {
			// no upgrade scheduled
			continue
		}
		// Inspect each pod, starting from the highest ordinal, and decrement the idx to allow
		// pod upgrades to go through, controlled by the StatefulSet controller.
		for idx := sset.GetReplicas(statefulSet) - 1; idx >= 0; idx-- {
			// Do we need to upgrade that pod?
			podName := sset.PodName(statefulSet.Name, idx)
			podRef := types.NamespacedName{Namespace: statefulSet.Namespace, Name: podName}
			// retrieve pod to inspect its revision label
			var pod corev1.Pod
			err := client.Get(context.Background(), podRef, &pod)
			if err != nil && !apierrors.IsNotFound(err) {
				return toUpgrade, err
			}
			if apierrors.IsNotFound(err) {
				// Pod does not exist, continue the loop as the absence will be accounted by the deletion driver
				continue
			}
			if sset.PodRevision(pod) != statefulSet.Status.UpdateRevision {
				toUpgrade = append(toUpgrade, pod)
			}
		}
	}
	return toUpgrade, nil
}

func doFlush(ctx context.Context, es esv1.Elasticsearch, esClient esclient.Client) error {
	targetEsVersion, err := version.Parse(es.Spec.Version)
	if err != nil {
		return err
	}

	switch {
	case targetEsVersion.Major >= 8:
		// Starting version 8.0, synced flush is not necessary anymore. A normal flush should be used instead.
		// During an upgrade from 7.x to 8.x we may have at least one Pod running 8.x already,
		// hence we check the target version here and not the currently running version.
		// It's ok to run a standard flush before 8.x, just not as optimal.
		log.Info("Requesting a flush", "es_name", es.Name, "namespace", es.Namespace)
		return esClient.Flush(ctx)
	default:
		// Pre 8.0, we should perform a synced flush (best-effort).
		log.Info("Requesting a synced flush", "es_name", es.Name, "namespace", es.Namespace)
		err := esClient.SyncedFlush(ctx)
		if esclient.IsConflict(err) {
			// Elasticsearch returns an error if the synced flush fails due to concurrent indexing operations.
			// The HTTP status code in that case will be 409 CONFLICT. We ignore that and consider synced flush best effort.
			log.Info("synced flush failed with 409 CONFLICT. Ignoring.", "namespace", es.Namespace, "es_name", es.Name)
			return nil
		}
		return err
	}
}

func (d *defaultDriver) maybeCompleteNodeUpgrades(
	ctx context.Context,
	esClient esclient.Client,
	esState ESState,
	nodeShutdown *shutdown.NodeShutdown,
) *reconciler.Results {
	// we still have to enable shard allocation in cases where we just upgraded from
	// a version that did not support node shutdown to a supported version.
	results := d.maybeEnableShardsAllocation(ctx, esClient, esState)
	if !results.HasError() && supportsNodeShutdown(esClient.Version()) {
		// clear all shutdowns of type restart that have completed
		// this relies on the fact the maybeEnableShardsAllocation checks expectations
		err := nodeShutdown.Clear(ctx, &esclient.ShutdownComplete)
		if err != nil {
			results = results.WithError(err)
		}
	}
	return results
}

func (d *defaultDriver) maybeEnableShardsAllocation(
	ctx context.Context,
	esClient esclient.Client,
	esState ESState,
) *reconciler.Results {
	results := &reconciler.Results{}
	// we are fully migrated to node shutdown and do not need this logic anymore
	if d.ReconcileState.OrchestrationHints().NoTransientSettings {
		return results
	}

	alreadyEnabled, err := esState.ShardAllocationsEnabled()
	if err != nil {
		return results.WithError(err)
	}
	if alreadyEnabled {
		return results
	}

	// Make sure all pods scheduled for upgrade have been upgraded.
	done, reason, err := d.expectationsSatisfied()
	if err != nil {
		return results.WithError(err)
	}
	if !done {
		reason := fmt.Sprintf("Enabling shards allocation: %s", reason)
		return results.WithReconciliationState(defaultRequeue.WithReason(reason))
	}

	statefulSets, err := sset.RetrieveActualStatefulSets(d.Client, k8s.ExtractNamespacedName(&d.ES))
	if err != nil {
		return results.WithError(err)
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
		return results.WithReconciliationState(defaultRequeue.WithReason("Nodes upgrade: some nodes are not back in the cluster yet"))
	}

	log.Info("Enabling shards allocation", "namespace", d.ES.Namespace, "es_name", d.ES.Name)
	if err := esClient.EnableShardAllocation(ctx); err != nil {
		return results.WithError(err)
	}
	return results
}

func (ctx *upgradeCtx) readyToDelete(pod corev1.Pod) (bool, error) {
	if !supportsNodeShutdown(ctx.esClient.Version()) {
		return true, nil // always OK to restart pre node shutdown support
	}
	if !k8s.IsPodReady(pod) {
		// there is no point in trying to query the shutdown status of a Pod that is not ready
		return true, nil
	}
	response, err := ctx.nodeShutdown.ShutdownStatus(ctx.parentCtx, pod.Name)
	if err != nil {
		return false, err
	}
	return response.Status == esclient.ShutdownComplete, nil
}

func (ctx *upgradeCtx) requestNodeRestarts(podsToRestart []corev1.Pod) error {
	var podNames []string //nolint:prealloc
	for _, p := range podsToRestart {
		if !k8s.IsPodReady(p) {
			// There is no point in trying to shut down a Pod that is not running.
			// Basing this off of the cached Kubernetes client's world view opens up a few edge
			// cases where a Pod might in fact already be running but the client's cache is not yet
			// up to date. But the trade-off made here i.e. accepting an ungraceful shutdown in these
			// edge case vs. being able to automatically unblock configuration rollouts that are blocked
			// due to misconfiguration, for example unfulfillable node selectors, seems worth it.
			continue
		}
		podNames = append(podNames, p.Name)
	}
	// Note that ReconcileShutdowns would cancel ongoing shutdowns when called with no podNames
	// this is however not the case in the rolling upgrade logic where we exit early if no pod needs to be rotated.
	return ctx.nodeShutdown.ReconcileShutdowns(ctx.parentCtx, podNames)
}

func (ctx *upgradeCtx) prepareClusterForNodeRestart(podsToUpgrade []corev1.Pod) error {
	// use client.Version here as we want the minimal version in the cluster not the one in the spec.
	if supportsNodeShutdown(ctx.esClient.Version()) {
		return ctx.requestNodeRestarts(podsToUpgrade)
	}
	// Disable shard allocations to avoid shards moving around while the node is temporarily down
	shardsAllocationEnabled, err := ctx.esState.ShardAllocationsEnabled()
	if err != nil {
		return err
	}
	if shardsAllocationEnabled {
		log.Info("Disabling shards allocation", "es_name", ctx.ES.Name, "namespace", ctx.ES.Namespace)
		if err := ctx.esClient.DisableReplicaShardsAllocation(ctx.parentCtx); err != nil {
			return err
		}
	}

	// Request a flush to optimize indices recovery when the node restarts.
	return doFlush(ctx.parentCtx, ctx.ES, ctx.esClient)
}
