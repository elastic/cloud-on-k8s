// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"fmt"
	"maps"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	umaps "github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

type poolParameters struct {
	SetDefaultSecurityContext bool
	OperatorNamespace         string

	Role           kblabel.Role
	Config         CanonicalConfig
	SecretName     string
	DeploymentName string
	ScaleToZero    bool
	BasePath       string
}

func newPoolParameters(operatorParameters operator.Parameters, kb *kbv1.Kibana, role kblabel.Role, basePath string, scaleToZero bool) (poolParameters, error) {
	var specConfigData map[string]any
	if kb.Spec.Config != nil {
		specConfigData = kb.Spec.Config.Data
	}
	cfg, err := settings.NewCanonicalConfigFrom(specConfigData)
	if err != nil {
		return poolParameters{}, err
	}

	// Single all-roles pool or UI pool: use the base config and base names.
	pp := poolParameters{
		SetDefaultSecurityContext: operatorParameters.SetDefaultSecurityContext,
		OperatorNamespace:         operatorParameters.OperatorNamespace,
		Role:                      role,
		BasePath:                  basePath,
		ScaleToZero:               scaleToZero,
		Config:                    CanonicalConfig{cfg},
		SecretName:                kbv1.ConfigSecret(kb.Name),
		DeploymentName:            kbv1.KBNamer.Suffix(kb.Name),
	}

	if !role.IsBackgroundTasks() {
		return pp, nil
	}

	pp.DeploymentName = kbv1.BackgroundTasksDeployment(kb.Name)
	// Only create a separate secret when there is an overlay, otherwise share the base secret.
	// kb.Spec.BackgroundTasks is non-nil when the background tasks role is active.
	if overlay := kb.Spec.BackgroundTasks.Config; overlay != nil {
		overlayCfg, err := settings.NewCanonicalConfigFrom(overlay.Data)
		if err != nil {
			return poolParameters{}, err
		}
		// The base config was built fresh above, so it can be merged in place.
		if err := pp.Config.MergeWith(overlayCfg); err != nil {
			return poolParameters{}, err
		}
		pp.SecretName = kbv1.BackgroundTasksConfigSecret(kb.Name)
	}

	return pp, nil
}

func reconcileService(ctx context.Context, client k8s.Client, kb *kbv1.Kibana, meta metadata.Metadata) (*corev1.Service, error) {
	// Fetch the existing Service once: its selector feeds the expected selector computation,
	// and it is reused for the orchestration-pause check below.
	var existing corev1.Service
	err := client.Get(ctx, types.NamespacedName{Namespace: kb.Namespace, Name: kbv1.HTTPService(kb.Name)}, &existing)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	svcExists := err == nil

	expectedSelector := serviceSelector(kb, existing.Spec.Selector)

	// The Service selector must follow the same migration as the Deployment selector: while the
	// primary Deployment still uses the legacy selector (no role label), some of its running pods
	// may not carry the role label yet. Switching the Service to role=primary at that point would
	// drop those pods from the endpoints while they are still healthy (a single-replica Kibana
	// would lose all endpoints). The Deployment selector is the signal: it only carries the role
	// label once the Deployment has been recreated, after all its pods were rolled with the label.
	if svcExists && !maps.Equal(existing.Spec.Selector, expectedSelector) {
		hasTheRole, err := primaryDeploymentSelectorHasRole(ctx, client, kb)
		if err != nil {
			return nil, err
		}
		if !hasTheRole {
			expectedSelector = existing.Spec.Selector
		}
	}

	expectedService := NewService(*kb, meta, expectedSelector)

	if svcExists && common.IsOrchestrationPaused(kb) && !maps.Equal(existing.Spec.Selector, expectedService.Spec.Selector) {
		ulog.FromContext(ctx).Info(
			"Service selector change detected but orchestration is paused; skipping Service reconciliation",
			"namespace", kb.Namespace, "kibana_name", kb.Name, "service", expectedService.Name,
		)
		return &existing, nil
	}

	return common.ReconcileService(ctx, client, expectedService, kb)
}

// primaryDeploymentSelectorHasRole returns false while the primary Deployment exists with a
// selector that does not carry the role label, i.e. while its selector migration is still in
// progress. When the Deployment does not exist (nothing deployed yet, or the migration just
// deleted it for recreation), it returns true: every pod the recreated Deployment adopts
// already carries the role label.
func primaryDeploymentSelectorHasRole(ctx context.Context, c k8s.Client, kb *kbv1.Kibana) (bool, error) {
	var dp appsv1.Deployment
	err := c.Get(ctx, types.NamespacedName{Namespace: kb.Namespace, Name: kbv1.KBNamer.Suffix(kb.Name)}, &dp)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return true, err
	}
	if dp.Spec.Selector == nil {
		return true, nil
	}
	_, hasRole := dp.Spec.Selector.MatchLabels[kblabel.RoleLabelName]
	return hasRole, nil
}

type poolDeploymentResult string

const (
	requeue poolDeploymentResult = "requeue"
	paused  poolDeploymentResult = "paused"
)

func (d *driver) reconcilePoolDeployment(
	ctx context.Context,
	kb *kbv1.Kibana,
	poolParams poolParameters,
	podAnnotations map[string]string,
	meta metadata.Metadata,
	reconciledSecrets map[string]struct{},
) (poolDeploymentResult, *appsv1.Deployment, error) {
	var existingDeployment appsv1.Deployment
	var existingSelector map[string]string

	if err := d.client.Get(ctx, types.NamespacedName{Namespace: kb.Namespace, Name: poolParams.DeploymentName}, &existingDeployment); err == nil {
		if existingDeployment.Spec.Selector != nil {
			existingSelector = existingDeployment.Spec.Selector.MatchLabels
		}
	} else if !apierrors.IsNotFound(err) {
		return "", nil, err
	}

	expectedSelector := deploymentSelector(kb, poolParams.Role, existingSelector)
	selectorOutcome, err := d.handleSelectorChange(ctx, kb, &existingDeployment, existingSelector, expectedSelector)
	if err != nil {
		return "", nil, err
	}
	if selectorOutcome.stopDeploymentReconciliation {
		return selectorOutcome.result, nil, nil
	}

	// reconcile pool secret, unless it was already written earlier in this reconciliation pass
	if _, exists := reconciledSecrets[poolParams.SecretName]; !exists {
		// Telemetry is only preserved in the base config secret, which this pool writes
		// unless it is a background pool with its own config overlay.
		keepTelemetry := poolParams.SecretName == kbv1.ConfigSecret(kb.Name)
		err := ReconcileConfigSecret(ctx, d.client, *kb, poolParams.Config, poolParams.SecretName, keepTelemetry, meta)
		if err != nil {
			return "", nil, err
		}
		reconciledSecrets[poolParams.SecretName] = struct{}{}
	}

	// Determine replica count for this pool.
	replicas, replicasToSave := deploymentReplicas(ctx, kb, poolParams, &existingDeployment)

	dpParams, err := d.deploymentParams(ctx, kb, poolParams, replicas, replicasToSave, podAnnotations, meta, selectorOutcome)
	if err != nil {
		return "", nil, err
	}

	expectedDp := deployment.New(dpParams)
	reconciledDp, err := common.ReconcilePauseAware(ctx, d.client, d.recorder, expectedDp, kb, ReconcilePoolDeployment)
	if err != nil {
		return "", nil, err
	}

	return selectorOutcome.result, &reconciledDp, nil
}

// selectorChangeOutcome tells reconcilePoolDeployment how to continue after the existing and the expected
// selectors of a pool Deployment were compared.
type selectorChangeOutcome struct {
	// stopDeploymentReconciliation is true when the reconciliation ends here and result is returned as is.
	stopDeploymentReconciliation bool
	// result is returned by the reconciliation: right away when stop is true, otherwise once the Deployment is reconciled.
	result poolDeploymentResult

	// expectedSelector is the selector to reconcile the deployment with.
	expectedSelector map[string]string
	// revisionHistoryLimitOverride is used instead of the revisionHistoryLimit of the Kibana spec, when not nil.
	revisionHistoryLimitOverride *int32
	omitRoleSplit                bool
}

// handleSelectorChange handles the difference between the existing and the expected selector of a pool Deployment.
// The selector is immutable, so when it changes the Deployment has to be deleted (leaving its pods orphaned) and
// recreated, which can only be done once it is safe. Until then the reconciliation either stops or proceeds with
// the existing selector. When there is no difference (or no existing Deployment) the reconciliation proceeds as is.
func (d *driver) handleSelectorChange(
	ctx context.Context,
	kb *kbv1.Kibana,
	existingDeployment *appsv1.Deployment,
	existingSelector, expectedSelector map[string]string,
) (selectorChangeOutcome, error) {
	logger := ulog.FromContext(ctx)

	// no existing deployment, or its selector already matches
	if existingSelector == nil || umaps.IsSubset(expectedSelector, existingSelector) {
		return selectorChangeOutcome{expectedSelector: expectedSelector}, nil
	}

	if common.IsOrchestrationPaused(kb) {
		logger.Info(
			"Deployment selector mismatch detected but orchestration is paused; deferring deletion",
			"deployment", existingDeployment.Name,
		)
		return selectorChangeOutcome{stopDeploymentReconciliation: true, result: paused}, nil
	}

	// An orphan deletion keeps the Deployment around (with a deletionTimestamp) until the garbage collector
	// has released its ReplicaSets. Nothing more to do but wait: do not reconcile (update) a Deployment that is
	// going away, and do not delete it again.
	if !existingDeployment.DeletionTimestamp.IsZero() {
		logger.V(1).Info("Deployment is already being deleted; waiting for it to go away", "deployment", existingDeployment.Name)
		return selectorChangeOutcome{stopDeploymentReconciliation: true, result: requeue}, nil
	}

	// The deployment cannot be deleted (yet): some pods do not have the role label, or the old ReplicaSets
	// are not cleaned up. Keep the existing selector for this reconciliation: the selector is immutable, so
	// sending the new one would make the update fail. The pod template (which carries the role label) is
	// still updated, rolling the pods until they are all labeled.
	//
	// Since the selector has changed, we always requeue: either we are waiting for the pods to get the role label
	// or the old ReplicaSets to be cleaned up (before the deployment can be deleted), or the deployment was deleted
	// and we need to reconcile again to create the new one.
	outcome := selectorChangeOutcome{result: requeue, expectedSelector: existingSelector, omitRoleSplit: true}

	allLabeled, blockingPod, err := allPodsHaveRoleLabel(ctx, d.client, existingDeployment)
	if err != nil {
		return selectorChangeOutcome{}, err
	}

	// The old ReplicaSets that do not carry the role label cannot be adopted by the recreated Deployment
	// and would be left behind. Setting revisionHistoryLimit to 0 makes the Deployment controller delete
	// all the inactive ones. There is no direct signal for the cleanup, so this is best effort: the
	// controller prunes only while syncing a deployment whose stored status already observes the latest
	// generation, which happens on a second sync after the limit update. A steady deployment that has
	// observed the limit is taken as cleaned up, but deleting it before that second sync leaves the old
	// ReplicaSets (0 replicas, no role label) orphaned. Accepted risk: the impact is only some empty
	// ReplicaSets.
	if allLabeled && hasZeroRevisionHistoryLimit(existingDeployment) && deployment.IsSteady(existingDeployment) {
		result, err := d.deletePoolDeployment(ctx, kb, expectedSelector, existingDeployment)
		return selectorChangeOutcome{stopDeploymentReconciliation: true, result: result}, err
	}

	if !allLabeled {
		logger.Info("Waiting for all pods to carry the role label before recreating the Deployment", "deployment", existingDeployment.Name, "pod", blockingPod)
		return outcome, nil
	}

	// The cleanup is not done (or not set yet): reconcile the deployment with revisionHistoryLimit 0 and
	// wait. The recreated deployment gets the limit back from the Kibana spec.
	logger.V(1).Info("Waiting for the old ReplicaSets to be cleaned up (revisionHistoryLimit 0) before recreating the deployment", "deployment", existingDeployment.Name)
	outcome.revisionHistoryLimitOverride = new(int32(0))

	return outcome, nil
}

// deletePoolDeployment deletes the Deployment, leaving its pods orphaned so that the recreated
// Deployment can adopt them.
func (d *driver) deletePoolDeployment(
	ctx context.Context,
	kb *kbv1.Kibana,
	expectedSelector map[string]string,
	existingDeployment *appsv1.Deployment,
) (poolDeploymentResult, error) {
	logger := ulog.FromContext(ctx)

	logger.Info(
		"Deployment selector mismatch; deleting to allow recreation with updated selector",
		"deployment", existingDeployment.Name,
		"existing_selector", existingDeployment.Spec.Selector.MatchLabels,
		"expected_selector", expectedSelector,
	)
	// Orphan the dependents (ReplicaSets, and through them the Pods) instead of cascading the delete,
	// so that the recreated Deployment can adopt them.
	// The UID precondition ensures we only delete the Deployment we examined, not one recreated under the same name.
	uid := existingDeployment.UID
	if delErr := d.client.Delete(
		ctx,
		existingDeployment,
		client.PropagationPolicy(metav1.DeletePropagationOrphan),
		client.Preconditions{UID: &uid},
	); delErr != nil {
		switch {
		case apierrors.IsNotFound(delErr):
			// already gone, nothing to do
		case apierrors.IsConflict(delErr):
			// The UID precondition failed: the Deployment was replaced since we read it.
			// Not an error, requeue to examine the current one.
			logger.Info("Deployment changed while deleting (UID precondition failed); requeuing", "deployment", existingDeployment.Name)
			return requeue, nil
		default:
			return "", delErr
		}
	}

	k8s.EmitEvent(d.recorder, kb, corev1.EventTypeNormal, events.EventReasonUpgraded, events.EventActionDeploymentReconciliation,
		fmt.Sprintf("Deleted Deployment %s to apply updated selector", existingDeployment.Name))

	return requeue, nil
}

// deploymentSelector returns the label selector to use for a pool deployment.
//
// The base case is kb.GetPoolIdentityLabels(role). One exception exists for the ECK upgrade
// path: when the expected selector would add role=primary to a single-pool (non-split) cluster
// whose existing deployment does not yet carry that label, the role label is stripped from the
// result. This avoids an unnecessary delete+recreate for every Kibana that does not use
// spec.backgroundTasks on operator upgrade.
//
// existingMatchLabels is nil when the deployment does not exist yet; in that case the full
// expected selector is returned.
func deploymentSelector(kb *kbv1.Kibana, role kblabel.Role, existingMatchLabels map[string]string) map[string]string {
	expectedByRole := kb.GetPoolIdentityLabels(role)

	if existingMatchLabels == nil {
		return expectedByRole
	}

	// ECK upgrade case: expectedByRole wants role=primary, the existing deployment has no role
	// label at all, and split is not active. Strip the role label so the selectors match and
	// no delete+recreate is triggered. The selector can be updated on the next split toggle.
	if expectedByRole[kblabel.RoleLabelName] == kblabel.RolePrimaryValue &&
		existingMatchLabels[kblabel.RoleLabelName] != kblabel.RolePrimaryValue &&
		!kb.BackgroundTasksEnabled() {
		result := maps.Clone(expectedByRole)
		delete(result, kblabel.RoleLabelName)
		return result
	}

	return expectedByRole
}

func hasZeroRevisionHistoryLimit(dep *appsv1.Deployment) bool {
	return dep.Spec.RevisionHistoryLimit != nil && *dep.Spec.RevisionHistoryLimit == 0
}

// allPodsHaveRoleLabel returns true if every pod selected by the given Deployment already carries the
// Kibana role label. It returns false otherwise, along with the name of the first pod missing the label.
// Pods in the Failed or Succeeded phase are ignored: they are not restarted in place (the Deployment
// controller replaces them) and may linger until pod garbage collection (e.g. after an eviction), so a
// terminated pod from before the upgrade must not block the migration. The recreated Deployment does not
// select them, so they are left alone.
func allPodsHaveRoleLabel(ctx context.Context, c k8s.Client, dep *appsv1.Deployment) (bool, string, error) {
	if dep.Spec.Selector == nil {
		return false, "", nil
	}

	var pods corev1.PodList
	if err := c.List(
		ctx,
		&pods,
		client.InNamespace(dep.Namespace),
		client.MatchingLabels(dep.Spec.Selector.MatchLabels),
	); err != nil {
		return false, "", err
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			continue
		}
		if _, ok := pod.Labels[kblabel.RoleLabelName]; !ok {
			return false, pod.Name, nil
		}
	}
	return true, "", nil
}

// serviceSelector returns the label selector to use for the Kibana HTTP Service, given the
// selector of the existing Service (nil when the Service does not exist yet). It applies the
// same ECK upgrade-path exception as deploymentSelector: when the existing Service has no role
// label yet and split mode is inactive, the role label is omitted so that the Service is not
// needlessly updated on every reconcile during an operator upgrade.
// kblabel.UIRole is used directly: the HTTP service is only created for the UI role and only
// ever targets the primary pool.
func serviceSelector(kb *kbv1.Kibana, existingSelector map[string]string) map[string]string {
	return deploymentSelector(kb, kblabel.UIRole, existingSelector)
}

func deploymentReplicas(ctx context.Context, kb *kbv1.Kibana, pm poolParameters, existingDeployment *appsv1.Deployment) (replicas *int32, replicasToSave *int32) {
	logger := ulog.FromContext(ctx)

	// replicasFromAnnotation returns the replica count saved in the maintenance annotation
	// when present, and removes the annotation so the saved count is consumed only once.
	// During the stop phase the returned value is persisted back into the annotation by the
	// subsequent deployment reconciliation.
	replicasFromAnnotation := func() *int32 {
		if existingDeployment == nil {
			return nil
		}
		if v, exists := existingDeployment.Annotations[replicasAnnotationName]; exists {
			i, err := strconv.ParseInt(v, 10, 32)
			if err != nil {
				logger.Error(err, "ignoring malformed replicas annotation", "annotation", replicasAnnotationName, "value", v)
				return nil
			}

			return new(int32(i))
		}
		return nil
	}

	backgroundReplicasToSave := func() *int32 {
		if kb.Spec.BackgroundTasks.Count != nil {
			return kb.Spec.BackgroundTasks.Count
		}

		if existingDeployment != nil &&
			// Do not store zero in annotation when the mode is scale to zero.
			existingDeployment.Spec.Replicas != nil && *existingDeployment.Spec.Replicas > 0 {
			return existingDeployment.Spec.Replicas
		}

		return replicasFromAnnotation() // restore from annotation
	}

	// backgroundReplicas returns the replica count to set on the background tasks Deployment.
	// Without an explicit count the replicas are managed externally (e.g. by an HPA): nil is
	// returned so that the deployment reconciliation adopts the live value without including it
	// in the template hash, and an HPA scale event does not cause a Deployment write. The only
	// exception is the restore after the stop phase: the Deployment is at zero (or has no replica
	// count yet) and the saved count is present in the annotation.
	backgroundReplicas := func() *int32 {
		if kb.Spec.BackgroundTasks.Count != nil {
			return kb.Spec.BackgroundTasks.Count
		}

		var fromDeployment *int32
		if existingDeployment != nil {
			fromDeployment = existingDeployment.Spec.Replicas // get the HPA decided value
		}

		fromAnnotation := replicasFromAnnotation()
		if fromDeployment == nil {
			return fromAnnotation
		}

		if *fromDeployment == 0 && fromAnnotation != nil {
			return fromAnnotation
		}

		return nil // return nil to keep the HPA value
	}

	switch {
	case pm.ScaleToZero:
		replicas = new(int32(0)) // drain both pools before starting the new version

		// Return the replica count to save if the deployment is background tasks. It will be stored in annotation.
		if pm.Role.IsBackgroundTasks() {
			replicasToSave = backgroundReplicasToSave()
			if replicasToSave == nil {
				replicasToSave = new(int32(1))
			}
		}

	case kb.BackgroundTasksEnabled() && pm.Role.IsBackgroundTasks():
		replicas = backgroundReplicas()

	default:
		replicas = new(kb.Spec.Count)
	}

	return replicas, replicasToSave
}

// shouldScaleAllPoolsToZeroForUpgrade reports whether the controller must scale both pools to zero before starting
// the new version. It returns true when background task isolation is active and at least one
// running pod (including terminating pods) carries a stale version label.
//
// Single-pool Kibana does not need this: the Recreate strategy on one Deployment already
// terminates all old pods before starting new ones. With two separate Deployments the
// strategies are independent, so an explicit cross-pool stop phase is required.
func shouldScaleAllPoolsToZeroForUpgrade(kb *kbv1.Kibana, pods []corev1.Pod) bool {
	if !kb.BackgroundTasksEnabled() {
		return false
	}
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			continue
		}
		ver, ok := pod.Labels[kblabel.KibanaVersionLabelName]
		if !ok || ver != kb.Spec.Version {
			return true
		}
	}
	return false
}

// poolStatusEntry associates a reconciled pool with its computed deployment status.
type poolStatusEntry struct {
	role   kblabel.Role
	status commonv1.DeploymentStatus
}

// buildStatus writes the per-pool statuses and the top-level deployment status to state.
// The top-level aggregate is computed from the pool sub-statuses after they are written, so it
// is always consistent with the pools section: when called with a partial list of entries, the
// aggregate includes the carried-over status of the pools not present.
func buildStatus(state *State, kb *kbv1.Kibana, pools []poolStatusEntry) {
	if len(pools) == 0 {
		// Nothing was reconciled: keep the previous status untouched rather than
		// overwriting the top level with an empty aggregate.
		return
	}

	status := &state.Kibana.Status
	for _, pool := range pools {
		if pool.role.IsBackgroundTasks() {
			status.Pools.BackgroundTasks = new(pool.status)
		} else {
			status.Pools.Primary = pool.status
		}
	}
	if !kb.BackgroundTasksEnabled() {
		// Single pool: clear any leftover background pool status.
		status.Pools.BackgroundTasks = nil
	}

	// The pool statuses start from the previously persisted pool statuses, which do not carry
	// the top-level conditions (e.g. OrchestrationPaused set during the reconcile loop).
	// Preserve them while the top-level DeploymentStatus is overwritten below.
	conditions := status.DeploymentStatus.Conditions
	status.DeploymentStatus = status.Pools.Primary
	status.DeploymentStatus.Conditions = conditions

	if !kb.BackgroundTasksEnabled() {
		// Single pool: the pool status is the top-level status.
		return
	}

	// Aggregate the pool sub-statuses, so that the top level is always the sum of the pool
	// entries written next to it. The top-level selector targets all pools.
	if bg := status.Pools.BackgroundTasks; bg != nil {
		status.Count += bg.Count
		status.AvailableNodes += bg.AvailableNodes
		status.Health = calculateAggregateHealth(status.Pools.Primary.Health, bg.Health)
	}
	status.Selector = metav1.FormatLabelSelector(&metav1.LabelSelector{
		MatchLabels: kb.GetIdentityLabels(), // without the role label
	})
}

func calculateAggregateHealth(primary, background commonv1.DeploymentHealth) commonv1.DeploymentHealth {
	switch primary {
	case background:
		return primary
	case commonv1.RedHealth: // no UI pod serves requests
		return commonv1.RedHealth
	default:
		return commonv1.YellowHealth
	}
}

// ReconcilePoolDeployment creates or updates the given deployment for the specified owner.
func ReconcilePoolDeployment(
	ctx context.Context,
	k8sClient k8s.Client,
	expected appsv1.Deployment,
	owner client.Object,
) (appsv1.Deployment, error) {
	return deployment.ReconcileWithMetadataMergeStrategy(
		ctx,
		k8sClient,
		expected,
		owner,
		deployment.MetadataMergeStrategy{
			Labels: umaps.Merge,
			Annotations: func(reconciled, expected map[string]string) map[string]string {
				merged := umaps.Merge(reconciled, expected)

				// Do not merge ECK owned annotations.
				if _, exists := expected[replicasAnnotationName]; !exists {
					delete(merged, replicasAnnotationName)
				}

				return merged
			},
		},
	)
}
