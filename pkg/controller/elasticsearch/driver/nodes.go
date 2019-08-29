// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
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

	// check if actual StatefulSets and corresponding pods match our expectations before applying any change
	ok, err := d.expectationsMet(actualStatefulSets)
	if err != nil {
		return results.WithError(err)
	}
	if !ok {
		return results.WithResult(defaultRequeue)
	}

	expectedResources, err := expectedResources(d.Client, d.ES, observedState, keystoreResources)
	if err != nil {
		return results.WithError(err)
	}

	// Phase 1: apply expected StatefulSets resources and scale up.
	if err := HandleUpscaleAndSpecChanges(d.Client, d.ES, d.Scheme(), expectedResources, actualStatefulSets); err != nil {
		return results.WithError(err)
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
	downscaleRes := HandleDownscale(downscaleCtx, expectedResources.StatefulSets(), actualStatefulSets)
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

func expectedResources(
	k8sClient k8s.Client,
	es v1alpha1.Elasticsearch,
	observedState observer.State,
	keystoreResources *keystore.Resources,
) (nodespec.ResourcesList, error) {
	resources, err := nodespec.BuildExpectedResources(es, keystoreResources)
	if err != nil {
		return nil, err
	}

	// patch configs to consider zen1 minimum master nodes
	if err := zen1.SetupMinimumMasterNodesConfig(resources); err != nil {
		return nil, err
	}
	// patch configs to consider zen2 initial master nodes
	if err := zen2.SetupInitialMasterNodes(es, observedState, k8sClient, resources); err != nil {
		return nil, err
	}

	return resources, nil
}
