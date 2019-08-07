// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version/zen1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

func (d *defaultDriver) reconcileNodeSpecs(
	esReachable bool,
	esClient esclient.Client,
	reconcileState *reconcile.State,
	observedState observer.State,
	resourcesState reconcile.ResourcesState,
	keystoreResources *keystore.Resources,
) *reconciler.Results {
	results := &reconciler.Results{}

	actualStatefulSets, err := sset.RetrieveActualStatefulSets(d.Client, k8s.ExtractNamespacedName(&d.ES))
	if err != nil {
		return results.WithError(err)
	}

	if !d.Expectations.GenerationExpected(actualStatefulSets.ObjectMetas()...) {
		// Our cache of StatefulSets is out of date compared to previous reconciliation operations.
		// Continuing with the reconciliation at this point may lead to:
		// - errors on rejected sset updates (conflict since cached resource out of date): that's ok
		// - calling ES orchestration settings (zen1/zen2/allocation excludes) with wrong assumptions: that's not ok
		// Hence we choose to abort the reconciliation early: will run again later with an updated cache.
		log.V(1).Info("StatefulSet cache out-of-date, re-queueing", "namespace", d.ES.Namespace, "es_name", d.ES.Name)
		return results.WithResult(defaultRequeue)
	}

	nodeSpecResources, err := nodespec.BuildExpectedResources(d.ES, keystoreResources)
	if err != nil {
		return results.WithError(err)
	}

	// TODO: there is a split brain possibility here if going from 1 to 3 masters or 3 to 7.
	//  See https://github.com/elastic/cloud-on-k8s/issues/1281.

	// patch configs to consider zen1 minimum master nodes
	if err := zen1.SetupMinimumMasterNodesConfig(nodeSpecResources); err != nil {
		return results.WithError(err)
	}
	// patch configs to consider zen2 initial master nodes
	if err := zen2.SetupInitialMasterNodes(d.ES, observedState, d.Client, nodeSpecResources); err != nil {
		return results.WithError(err)
	}

	// Phase 1: apply expected StatefulSets resources, but don't scale down.
	// The goal is to:
	// 1. scale sset up (eg. go from 3 to 5 replicas).
	// 2. apply configuration changes on the sset resource, to be used for future pods creation/recreation,
	//    but do not rotate pods yet.
	// 3. do **not** apply replicas scale down, otherwise nodes would be deleted before
	//    we handle a clean deletion.
	for _, nodeSpecRes := range nodeSpecResources {
		// always reconcile config (will apply to new & recreated pods)
		if err := settings.ReconcileConfig(d.Client, d.ES, nodeSpecRes.StatefulSet.Name, nodeSpecRes.Config); err != nil {
			return results.WithError(err)
		}
		if _, err := common.ReconcileService(d.Client, d.Scheme, &nodeSpecRes.HeadlessService, &d.ES); err != nil {
			return results.WithError(err)
		}
		ssetToApply := *nodeSpecRes.StatefulSet.DeepCopy()
		actual, exists := actualStatefulSets.GetByName(ssetToApply.Name)
		if exists && sset.Replicas(ssetToApply) < sset.Replicas(actual) {
			// sset needs to be scaled down
			// update the sset to use the new spec but don't scale replicas down for now
			ssetToApply.Spec.Replicas = actual.Spec.Replicas
		}
		if err := sset.ReconcileStatefulSet(d.Client, d.Scheme, d.ES, ssetToApply); err != nil {
			return results.WithError(err)
		}
	}

	if !esReachable {
		// Cannot perform next operations if we cannot request Elasticsearch.
		log.Info("ES external service not ready yet for further reconciliation, re-queuing.", "namespace", d.ES.Namespace, "es_name", d.ES.Name)
		reconcileState.UpdateElasticsearchPending(resourcesState.CurrentPods)
		return results.WithResult(defaultRequeue)
	}

	// Update Zen1 minimum master nodes through the API, corresponding to the current nodes we have.
	requeue, err := zen1.UpdateMinimumMasterNodes(d.Client, d.ES, esClient, actualStatefulSets, reconcileState)
	if err != nil {
		return results.WithError(err)
	}
	if requeue {
		results.WithResult(defaultRequeue)
	}
	// Maybe clear zen2 voting config exclusions.
	requeue, err = zen2.ClearVotingConfigExclusions(d.ES, d.Client, esClient, actualStatefulSets)
	if err != nil {
		return results.WithError(err)
	}
	if requeue {
		results.WithResult(defaultRequeue)
	}

	// Phase 2: handle sset scale down.
	// We want to safely remove nodes from the cluster, either because the sset requires less replicas,
	// or because it should be removed entirely.
	downscaleCtx := downscaleContext{
		k8sClient:      d.Client,
		esClient:       esClient,
		resourcesState: resourcesState,
		observedState:  observedState,
		reconcileState: reconcileState,
		es:             d.ES,
		expectations:   d.Expectations,
	}
	downscaleRes := HandleDownscale(downscaleCtx, nodeSpecResources.StatefulSets(), actualStatefulSets)
	results.WithResults(downscaleRes)
	if downscaleRes.HasError() {
		return results
	}

	// Phase 3: handle rolling upgrades.
	// Control nodes restart (upgrade) by manually decrementing rollingUpdate.Partition.
	rollingUpgradesRes := d.handleRollingUpgrades(esClient, actualStatefulSets)
	results.WithResults(rollingUpgradesRes)
	if rollingUpgradesRes.HasError() {
		return results
	}

	// TODO:
	//  - change budget
	//  - grow and shrink
	return results
}
