// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/pdb"
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
	certResources *certificates.CertificateResources,
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

	expectedResources, err := nodespec.BuildExpectedResources(d.ES, keystoreResources, d.Scheme(), certResources)
	if err != nil {
		return results.WithError(err)
	}

	if err := GarbageCollectPVCs(d.K8sClient(), d.ES, actualStatefulSets, expectedResources.StatefulSets()); err != nil {
		return results.WithError(err)
	}

	esState := NewMemoizingESState(esClient)

	// Phase 1: apply expected StatefulSets resources and scale up.
	upscaleCtx := upscaleCtx{
		k8sClient:     d.K8sClient(),
		es:            d.ES,
		scheme:        d.Scheme(),
		observedState: observedState,
		esState:       esState,
		expectations:  d.Expectations,
	}
	actualStatefulSets, err = HandleUpscaleAndSpecChanges(upscaleCtx, actualStatefulSets, expectedResources)
	if err != nil {
		return results.WithError(err)
	}

	// Update PDB to account for new replicas.
	if err := pdb.Reconcile(d.Client, d.Scheme(), d.ES, actualStatefulSets); err != nil {
		return results.WithError(err)
	}

	if !esReachable {
		// Cannot perform next operations if we cannot request Elasticsearch.
		log.Info("ES external service not ready yet for further reconciliation, re-queuing.", "namespace", d.ES.Namespace, "es_name", d.ES.Name)
		reconcileState.UpdateElasticsearchApplyingChanges(resourcesState.CurrentPods)
		return results.WithResult(defaultRequeue)
	}

	// Update Zen1 minimum master nodes through the API, corresponding to the current nodes we have.
	requeue, err := zen1.UpdateMinimumMasterNodes(d.Client, d.ES, esClient, actualStatefulSets)
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
	downscaleCtx := newDownscaleContext(
		d.Client,
		esClient,
		resourcesState,
		observedState,
		reconcileState,
		d.Expectations,
		d.ES,
	)
	downscaleRes := HandleDownscale(downscaleCtx, expectedResources.StatefulSets(), actualStatefulSets)
	results.WithResults(downscaleRes)
	if downscaleRes.HasError() {
		return results
	}

	// Phase 3: handle rolling upgrades.
	rollingUpgradesRes := d.handleRollingUpgrades(esClient, esState, actualStatefulSets, expectedResources.MasterNodesNames())
	results.WithResults(rollingUpgradesRes)
	if rollingUpgradesRes.HasError() {
		return results
	}

	// When not reconciled, set the phase to ApplyingChanges only if it was Ready to avoid to
	// override another "not Ready" phase like MigratingData.
	if Reconciled(expectedResources.StatefulSets(), actualStatefulSets, d.Client) {
		reconcileState.UpdateElasticsearchReady(resourcesState, observedState)
	} else if reconcileState.IsElasticsearchReady(observedState) {
		reconcileState.UpdateElasticsearchApplyingChanges(resourcesState.CurrentPods)
	}

	// TODO:
	//  - change budget
	//  - grow and shrink
	return results
}

// Reconciled reports whether the actual StatefulSets are reconciled to match the expected StatefulSets
// by checking that the expected template hash label is reconciled for all StatefulSets, there are no
// pod upgrades in progress and all pods are running.
func Reconciled(expectedStatefulSets, actualStatefulSets sset.StatefulSetList, client k8s.Client) bool {
	// actual sset should have the expected sset template hash label
	for _, expectedSset := range expectedStatefulSets {
		actualSset, ok := actualStatefulSets.GetByName(expectedSset.Name)
		if !ok {
			return false
		}
		if !sset.EqualTemplateHashLabels(expectedSset, actualSset) {
			log.V(1).Info("Statefulset not reconciled",
				"statefulset_name", expectedSset.Name, "reason", "template hash not equal")
			return false
		}
	}

	// all pods should have been upgraded
	pods, err := podsToUpgrade(client, actualStatefulSets)
	if err != nil {
		return false
	}
	if len(pods) > 0 {
		log.V(1).Info("Statefulset not reconciled", "reason", "pod not upgraded")
		return false
	}

	return true
}
