// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package stateful implements the stateful Elasticsearch driver using StatefulSets.
package stateful

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	commondriver "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/shared"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/pdb"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/shutdown"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// Driver is the stateful Elasticsearch driver implementation using StatefulSets.
type Driver struct {
	driver.BaseDriver
}

// NewDriver returns a new stateful driver implementation.
func NewDriver(parameters driver.Parameters) driver.Driver {
	return &Driver{BaseDriver: driver.BaseDriver{Parameters: parameters}}
}

var _ commondriver.Interface = (*Driver)(nil)

// Reconcile fulfills the Driver interface and reconciles the cluster resources.
func (d *Driver) Reconcile(ctx context.Context) *reconciler.Results {
	results := reconciler.NewResult(ctx)

	enterpriseFeaturesEnabled, err := d.LicenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return results.WithError(err)
	}

	// Resolve configuration first. This computes merged configs for all NodeSets
	// (including StackConfigPolicy) and detects clientAuthenticationRequired early,
	// before we create the ES client.
	resolvedConfig, err := ResolveConfig(ctx, d.Client, d.ES, d.OperatorParameters.IPFamily, enterpriseFeaturesEnabled)
	if err != nil {
		return results.WithError(err)
	}

	if resolvedConfig.ClientAuthenticationOverrideWarning != "" {
		d.ReconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonValidation, events.EventActionValidation, resolvedConfig.ClientAuthenticationOverrideWarning)
	}

	// Reconcile resources which are common to all drivers.
	sharedState, sharedResults := shared.ReconcileSharedResources(ctx, d, d.Parameters, resolvedConfig.ClientAuthenticationRequired)
	if sharedResults.HasError() {
		return results.WithResults(sharedResults)
	}
	defer sharedState.ESClient.Close()
	results.WithResults(sharedResults)

	// File settings (stateful: SCP-deferred empty secret creation).
	// Stateless clusters manage the file-settings Secret themselves via
	// filesettings.ReconcileClusterSecrets; this call is stateful-only.
	if d.Version.GTE(filesettings.FileBasedSettingsMinPreVersion) {
		requeue, err := shared.MaybeReconcileEmptyFileSettingsSecret(ctx, d.Client, d.LicenseChecker, &d.ES, d.OperatorParameters.OperatorNamespace)
		if err != nil {
			return results.WithError(err)
		} else if requeue {
			results.WithReconciliationState(
				shared.DefaultRequeue.WithReason(
					fmt.Sprintf("This cluster is targeted by at least one StackConfigPolicy, expecting Secret %s to be created by StackConfigPolicy controller",
						esv1.FileSettingsSecretName(d.ES.Name)),
				),
			)
		}
	}

	// Keystore (stateful: init container + volume, optional managed password Secret).
	// Stateless clusters don't use the init container keystore — they deliver secure
	// settings via cluster_secrets in file-based settings.
	keystoreResources, err := d.reconcileKeystore(ctx, sharedState.Meta)
	if err != nil {
		return results.WithError(err)
	}

	// Stateful specific: Service accounts hint
	results.WithError(d.maybeSetServiceAccountsOrchestrationHint(
		ctx, sharedState.ESReachable, sharedState.ESClient, sharedState.ResourcesState))

	// Stateful specific: Suspended pods
	// We want to reconcile suspended Pods before we start reconciling node specs as this is considered a debugging and
	// troubleshooting tool that does not follow the change budget restrictions
	if err := reconcileSuspendedPods(ctx, d.Client, d.ES, d.Expectations); err != nil {
		return results.WithError(err)
	}

	if common.IsOrchestrationPaused(&d.ES) {
		return results.WithResults(d.reconcileCriticalStepsWhilePaused(ctx, sharedState, resolvedConfig, keystoreResources))
	}

	d.maybeResetPausedCondition()

	// Stateful specific: Node specs (StatefulSets, upgrades, downscales)
	return results.WithResults(d.reconcileNodeSpecs(
		ctx, sharedState.ESReachable, sharedState.ESClient, d.ReconcileState,
		*sharedState.ResourcesState, keystoreResources, sharedState.Meta, resolvedConfig))
}

func (d *Driver) maybeResetPausedCondition() {
	orchestrationPausedIndex := d.ES.Status.Conditions.Index(commonv1alpha1.OrchestrationPaused)
	if orchestrationPausedIndex >= 0 {
		d.ReconcileState.ReportCondition(commonv1alpha1.OrchestrationPaused, corev1.ConditionFalse, "Orchestration has resumed normally")
	}
}

// reconcileCriticalStepsWhilePaused runs when pause-orchestration is enabled.
//
// Motivation (see https://github.com/elastic/cloud-on-k8s/issues/9250): during node-level maintenance,
// a concurrent Elasticsearch spec change can otherwise trigger rolling upgrades on top of the drain,
// compounding disruption. Pausing orchestration narrows the freeze to spec-driven StatefulSet work
// instead of using managed=false, which would skip all reconciliation (including certs and discovery)
// and risk cluster degradation over long windows.
//
// Shared reconciliation (certs, services, users, health, etc.) has already completed in Reconcile;
// this path skips reconcileNodeSpecs but still performs StatefulSet-adjacent work: wait on expectations,
// reconcile the PDB, clear completed restart shutdown records when supported, detect pending spec changes
// versus the live cluster, and report OrchestrationPaused (with a warning event and periodic requeue
// when changes are pending).
func (d *Driver) reconcileCriticalStepsWhilePaused(
	ctx context.Context,
	state *shared.ReconcileState,
	resolvedConfig nodespec.ResolvedConfig,
	keystoreResources *keystore.Resources,
) *reconciler.Results {
	defer tracing.Span(&ctx)()
	log := ulog.FromContext(ctx)
	results := &reconciler.Results{}
	done, reason, err := d.expectationsSatisfied(ctx)
	if err != nil {
		return results.WithError(err)
	}
	if !done {
		return results.WithReconciliationState(shared.DefaultRequeue.WithReason(reason))
	}

	actualSets, err := es_sset.RetrieveActualStatefulSets(d.Client, k8s.ExtractNamespacedName(&d.ES))
	if err != nil {
		return results.WithError(err)
	}
	if err = pdb.Reconcile(ctx, d.Client, d.ES, d.OperatorParameters.OperatorNamespace, actualSets, state.Meta); err != nil {
		return results.WithError(err)
	}

	if supportsNodeShutdown(state.ESClient.Version()) {
		esState := NewMemoizingESState(ctx, state.ESClient)
		nodeNameToID, err := esState.NodeNameToID()
		if err != nil {
			return results.WithError(err)
		}
		nodeShutdown := shutdown.NewNodeShutdown(state.ESClient, nodeNameToID, esclient.Restart, "", nil, log)
		actualPods, err := es_sset.GetActualPodsForCluster(d.Client, d.ES)
		if err != nil {
			return results.WithError(err)
		}

		terminatingNodes := k8s.PodNames(k8s.TerminatingPods(actualPods))
		results = results.WithError(nodeShutdown.Clear(ctx,
			esclient.ShutdownComplete.Applies,
			nodeShutdown.OnlyNodesInCluster,
			nodeShutdown.OnlyNonTerminatingNodes(terminatingNodes),
		))
	}

	hasPendingChanges, err := d.hasPendingSpecChanges(ctx, actualSets, state, resolvedConfig, keystoreResources)
	if err != nil {
		return results.WithError(err)
	}

	message := "Orchestration paused via annotation; no pending spec changes detected"
	if hasPendingChanges {
		message = "Orchestration paused via annotation; spec changes are pending and will be applied on resume"
		d.ReconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonPaused,
			events.EventActionPendingOrchestrationChanges, "Spec changes pending but orchestration is paused — remove annotation to apply")
		results.WithRequeue(5 * time.Minute)
	}
	d.ReconcileState.ReportCondition(commonv1alpha1.OrchestrationPaused, corev1.ConditionTrue, message)
	return results
}

func (d *Driver) hasPendingSpecChanges(
	ctx context.Context,
	actualSets es_sset.StatefulSetList,
	state *shared.ReconcileState,
	resolvedConfig nodespec.ResolvedConfig,
	keystoreResources *keystore.Resources,
) (bool, error) {
	expectedResources, err := nodespec.BuildExpectedResources(ctx, d.Client, d.ES, keystoreResources, actualSets,
		d.OperatorParameters.SetDefaultSecurityContext, state.Meta, resolvedConfig)
	if err != nil {
		return false, err
	}
	return hasSpecDiff(ctx, actualSets, expectedResources.StatefulSets()), nil
}

func hasSpecDiff(ctx context.Context, actualSets, expectedSets es_sset.StatefulSetList) bool {
	log := ulog.FromContext(ctx)

	if len(actualSets) != len(expectedSets) {
		log.V(1).Info("Different number of statefulsets found", "actual_count", len(actualSets), "expected_count", len(expectedSets))
		return true
	}

	otherSetsByName := make(map[string]appsv1.StatefulSet, len(expectedSets))
	for _, otherSet := range expectedSets {
		otherSetsByName[otherSet.Name] = otherSet
	}

	for _, thisSet := range actualSets {
		thatSet, exists := otherSetsByName[thisSet.Name]
		if !exists {
			ssetLogger(ctx, thisSet).V(1).Info("statefulset does not exist in other sets")
			return true
		}

		if !es_sset.EqualTemplateHashLabels(thatSet, thisSet) {
			ssetLogger(ctx, thisSet).V(1).Info("statefulset template hash differs",
				"expected_hash", hash.GetTemplateHashLabel(thatSet.Labels),
				"actual_hash", hash.GetTemplateHashLabel(thisSet.Labels))
			return true
		}
	}

	return false
}

// names returns the names of the given pods.
func names(pods []corev1.Pod) []string {
	result := make([]string, len(pods))
	for i, pod := range pods {
		result[i] = pod.Name
	}
	return result
}
