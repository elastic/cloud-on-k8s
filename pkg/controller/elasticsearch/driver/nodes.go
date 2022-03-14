// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"errors"
	"fmt"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates/transport"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/pdb"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func (d *defaultDriver) reconcileNodeSpecs(
	ctx context.Context,
	esReachable bool,
	esClient esclient.Client,
	reconcileState *reconcile.State,
	resourcesState reconcile.ResourcesState,
	keystoreResources *keystore.Resources,
) *reconciler.Results {
	span, ctx := apm.StartSpan(ctx, "reconcile_node_spec", tracing.SpanTypeApp)
	defer span.End()

	results := &reconciler.Results{}

	// If some nodeSets are managed by the autoscaler, wait for them to be updated.
	if ok, err := autoscaledResourcesSynced(d.ES); err != nil {
		return results.WithError(fmt.Errorf("StatefulSet recreation: %w", err))
	} else if !ok {
		return results.WithReconciliationState(defaultRequeue.WithReason("Waiting for autoscaling controller to sync node sets"))
	}

	// check if actual StatefulSets and corresponding pods match our expectations before applying any change
	ok, reason, err := d.expectationsSatisfied()
	if err != nil {
		return results.WithError(err)
	}
	if !ok {
		return results.WithReconciliationState(defaultRequeue.WithReason(reason))
	}

	// recreate any StatefulSet that needs to account for PVC expansion
	recreations, err := recreateStatefulSets(d.K8sClient(), d.ES)
	if err != nil {
		return results.WithError(fmt.Errorf("StatefulSet recreation: %w", err))
	}
	if recreations > 0 {
		// Some StatefulSets are in the process of being recreated to handle PVC expansion:
		// it is safer to requeue until the re-creation is done.
		// Otherwise, some operation could be performed with wrong assumptions:
		// the sset doesn't exist (was just deleted), but the Pods do actually exist.
		log.V(1).Info("StatefulSets recreation in progress, re-queueing.",
			"namespace", d.ES.Namespace, "es_name", d.ES.Name, "recreations", recreations)
		return results.WithReconciliationState(defaultRequeue.WithReason("StatefulSets recreation in progress"))
	}

	actualStatefulSets, err := sset.RetrieveActualStatefulSets(d.Client, k8s.ExtractNamespacedName(&d.ES))
	if err != nil {
		return results.WithError(err)
	}

	expectedResources, err := nodespec.BuildExpectedResources(d.Client, d.ES, keystoreResources, actualStatefulSets, d.OperatorParameters.IPFamily, d.OperatorParameters.SetDefaultSecurityContext)
	if err != nil {
		return results.WithError(err)
	}

	esState := NewMemoizingESState(ctx, esClient)
	// Phase 1: apply expected StatefulSets resources and scale up.
	upscaleCtx := upscaleCtx{
		parentCtx:            ctx,
		k8sClient:            d.K8sClient(),
		es:                   d.ES,
		esState:              esState,
		expectations:         d.Expectations,
		validateStorageClass: d.OperatorParameters.ValidateStorageClass,
		upscaleReporter:      reconcileState.UpscaleReporter,
	}
	upscaleResults, err := HandleUpscaleAndSpecChanges(upscaleCtx, actualStatefulSets, expectedResources)
	if err != nil {
		reconcileState.AddEvent(corev1.EventTypeWarning, events.EventReconciliationError, fmt.Sprintf("Failed to apply spec change: %v", err))
		var podTemplateErr *sset.PodTemplateError
		if errors.As(err, &podTemplateErr) {
			// An error has been detected in one of the pod templates, let's update the phase to "invalid"
			reconcileState.UpdateElasticsearchInvalidWithEvent(err.Error())
		}
		return results.WithError(err)
	}

	if upscaleResults.Requeue {
		return results.WithReconciliationState(defaultRequeue.WithReason("StatefulSet is scheduled for recreation"))
	}
	if reconcileState.HasPendingNewNodes() {
		results.WithReconciliationState(defaultRequeue.WithReason("Upscale in progress"))
	}
	actualStatefulSets = upscaleResults.ActualStatefulSets

	// Once all the StatefulSets have been updated we can ensure that the former version of the transport certificates Secret is deleted.
	if err := transport.DeleteLegacyTransportCertificate(d.Client, d.ES); err != nil {
		results.WithError(err)
	}

	// Update PDB to account for new replicas.
	if err := pdb.Reconcile(d.Client, d.ES, actualStatefulSets); err != nil {
		return results.WithError(err)
	}

	if err := reconcilePVCOwnerRefs(d.K8sClient(), d.ES); err != nil {
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
		reconcileState.UpdateWithPhase(esv1.ElasticsearchApplyingChangesPhase)
		return results.WithError(err)
	}

	// Next operations require the Elasticsearch API to be available.
	if !esReachable {
		msg := "Elasticsearch cannot be reached yet, re-queuing"
		log.Info(msg, "namespace", d.ES.Namespace, "es_name", d.ES.Name)
		reconcileState.UpdateWithPhase(esv1.ElasticsearchApplyingChangesPhase)
		return results.WithReconciliationState(defaultRequeue.WithReason(msg))
	}

	// Maybe update Zen1 minimum master nodes through the API, corresponding to the current nodes we have.
	requeue, err := zen1.UpdateMinimumMasterNodes(ctx, d.Client, d.ES, esClient, actualStatefulSets)
	if err != nil {
		return results.WithError(err)
	}
	if requeue {
		results.WithReconciliationState(defaultRequeue.WithReason("Not enough available masters to update Zen1 settings"))
	}
	// Remove the zen2 bootstrap annotation if bootstrap is over.
	requeue, err = zen2.RemoveZen2BootstrapAnnotation(ctx, d.Client, d.ES, esClient)
	if err != nil {
		return results.WithError(err)
	}
	if requeue {
		results.WithReconciliationState(defaultRequeue.WithReason("Initial cluster bootstrap is not complete"))
	}
	// Maybe clear zen2 voting config exclusions.
	requeue, err = zen2.ClearVotingConfigExclusions(ctx, d.ES, d.Client, esClient, actualStatefulSets)
	if err != nil {
		return results.WithError(fmt.Errorf("when clearing voting exclusions: %w", err))
	}
	if requeue {
		results.WithReconciliationState(defaultRequeue.WithReason("Cannot clear voting exclusions yet"))
	}
	// shutdown logic is dependent on Elasticsearch version
	nodeShutdowns, err := newShutdownInterface(d.ES, esClient, esState, reconcileState.StatusReporter)
	if err != nil {
		return results.WithError(err)
	}

	// Phase 2: handle sset scale down.
	// We want to safely remove nodes from the cluster, either because the sset requires less replicas,
	// or because it should be removed entirely.
	downscaleCtx := newDownscaleContext(
		ctx,
		d.Client,
		esClient,
		resourcesState,
		reconcileState,
		d.Expectations,
		d.ES,
		nodeShutdowns,
	)

	downscaleRes := HandleDownscale(downscaleCtx, expectedResources.StatefulSets(), actualStatefulSets)
	results.WithResults(downscaleRes)
	if downscaleRes.HasError() {
		return results
	}

	// Phase 3: handle rolling upgrades.
	rollingUpgradesRes := d.handleUpgrades(ctx, esClient, esState, expectedResources)
	results.WithResults(rollingUpgradesRes)
	if rollingUpgradesRes.HasError() {
		return results
	}

	// as of 7.15.2 with node shutdown we do not need transient settings anymore and in fact want to remove any left-overs.
	if esReachable && d.isNodeSpecsReconciled(actualStatefulSets, d.Client, results) {
		if err := d.maybeRemoveTransientSettings(ctx, esClient); err != nil {
			return results.WithError(err)
		}
	}

	return results
}

func (d *defaultDriver) isNodeSpecsReconciled(
	actualStatefulSets sset.StatefulSetList,
	client k8s.Client,
	result *reconciler.Results,
) bool {
	if isReconciled, _ := result.IsReconciled(); !isReconciled {
		return false
	}
	if satisfied, _, err := d.Expectations.Satisfied(); err != nil || !satisfied {
		return false
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
