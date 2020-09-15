// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"context"
	"errors"
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/pdb"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
)

func (d *defaultDriver) reconcileNodeSpecs(
	ctx context.Context,
	esReachable bool,
	esClient esclient.Client,
	reconcileState *reconcile.State,
	observedState observer.State,
	resourcesState reconcile.ResourcesState,
	keystoreResources *keystore.Resources,
) *reconciler.Results {
	span, ctx := apm.StartSpan(ctx, "reconcile_node_spec", tracing.SpanTypeApp)
	defer span.End()

	results := &reconciler.Results{}

	// check if actual StatefulSets and corresponding pods match our expectations before applying any change
	ok, err := d.expectationsSatisfied()
	if err != nil {
		return results.WithError(err)
	}
	if !ok {
		return results.WithResult(defaultRequeue)
	}

	actualStatefulSets, err := sset.RetrieveActualStatefulSets(d.Client, k8s.ExtractNamespacedName(&d.ES))
	if err != nil {
		return results.WithError(err)
	}

	expectedResources, err := nodespec.BuildExpectedResources(d.ES, keystoreResources, actualStatefulSets, d.OperatorParameters.IPFamily, d.OperatorParameters.SetDefaultSecurityContext)
	if err != nil {
		return results.WithError(err)
	}

	esState := NewMemoizingESState(ctx, esClient)

	// Phase 1: apply expected StatefulSets resources and scale up.
	upscaleCtx := upscaleCtx{
		parentCtx:     ctx,
		k8sClient:     d.K8sClient(),
		es:            d.ES,
		observedState: observedState,
		esState:       esState,
		expectations:  d.Expectations,
	}
	upscaleResults, err := HandleUpscaleAndSpecChanges(upscaleCtx, actualStatefulSets, expectedResources)
	if err != nil {
		reconcileState.AddEvent(corev1.EventTypeWarning, events.EventReconciliationError, fmt.Sprintf("Failed to apply spec change: %v", err))
		var podTemplateErr *sset.PodTemplateError
		if errors.As(err, &podTemplateErr) {
			// An error has been detected in one of the pod templates, let's update the phase to "invalid"
			reconcileState.UpdateElasticsearchInvalid(err)
		}
		return results.WithError(err)
	}
	if upscaleResults.Requeue {
		return results.WithResult(defaultRequeue)
	}
	actualStatefulSets = upscaleResults.ActualStatefulSets

	// Update PDB to account for new replicas.
	if err := pdb.Reconcile(d.Client, d.ES, actualStatefulSets); err != nil {
		return results.WithError(err)
	}

	if err := GarbageCollectPVCs(d.K8sClient(), d.ES, actualStatefulSets, expectedResources.StatefulSets()); err != nil {
		return results.WithError(err)
	}

	// Phase 2: if there is any Pending or bootlooping Pod to upgrade, do it.
	attempted, err := d.MaybeForceUpgrade(actualStatefulSets)
	if err != nil || attempted {
		// If attempted, we're in a transient state where it's safer to requeue.
		// We don't want to re-upgrade in a regular way the pods we just force-upgraded.
		// Next reconciliation will check expectations again.
		reconcileState.UpdateElasticsearchApplyingChanges(resourcesState.CurrentPods)
		return results.WithError(err)
	}

	// Next operations require the Elasticsearch API to be available.
	if !esReachable {
		log.Info("ES cannot be reached yet, re-queuing", "namespace", d.ES.Namespace, "es_name", d.ES.Name)
		reconcileState.UpdateElasticsearchApplyingChanges(resourcesState.CurrentPods)
		return results.WithResult(defaultRequeue)
	}

	// Maybe update Zen1 minimum master nodes through the API, corresponding to the current nodes we have.
	requeue, err := zen1.UpdateMinimumMasterNodes(ctx, d.Client, d.ES, esClient, actualStatefulSets)
	if err != nil {
		return results.WithError(err)
	}
	if requeue {
		results.WithResult(defaultRequeue)
	}
	// Remove the zen2 bootstrap annotation if bootstrap is over.
	requeue, err = zen2.RemoveZen2BootstrapAnnotation(ctx, d.Client, d.ES, esClient)
	if err != nil {
		return results.WithError(err)
	}
	if requeue {
		results.WithResult(defaultRequeue)
	}
	// Maybe clear zen2 voting config exclusions.
	requeue, err = zen2.ClearVotingConfigExclusions(ctx, d.ES, d.Client, esClient, actualStatefulSets)
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
		ctx,
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
	rollingUpgradesRes := d.handleRollingUpgrades(ctx, esClient, esState, expectedResources.MasterNodesNames())
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
