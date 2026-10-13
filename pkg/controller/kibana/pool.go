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
	OperatorParameters operator.Parameters

	Role           kblabel.Role
	Config         CanonicalConfig
	SecretName     string
	DeploymentName string
	ScaleToZero    bool
	BasePath       string
}

func poolParams(operatorParameters operator.Parameters, kb *kbv1.Kibana, role kblabel.Role, basePath string, scaleToZero bool) (params poolParameters, rErr error) {
	var specConfigData map[string]any
	if kb.Spec.Config != nil {
		specConfigData = kb.Spec.Config.Data
	}
	pcg, err := settings.NewCanonicalConfigFrom(specConfigData)
	if err != nil {
		return poolParameters{}, err
	}
	primaryPoolConfig := CanonicalConfig{pcg}

	pp := poolParameters{
		OperatorParameters: operatorParameters,
		Role:               role,
		BasePath:           basePath,
		ScaleToZero:        scaleToZero,
	}

	if role.IsSinglePool() || role.IsUI() || (role == kblabel.Role{}) {
		// Single all-roles pool or UI pool: use the base config and base names.
		pp.Config = primaryPoolConfig
		pp.SecretName = kbv1.ConfigSecret(kb.Name)
		pp.DeploymentName = kbv1.KBNamer.Suffix(kb.Name)
		return pp, nil
	}

	// Background tasks pool: only create a separate secret when there is an overlay.
	overlay := kb.Spec.BackgroundTasks.Config // kb.Spec.BackgroundTasks is non-nil when BG role is active
	if overlay == nil {
		// No overlay — share the base secret; no separate BG secret needed.
		pp.Config = primaryPoolConfig
		pp.SecretName = kbv1.ConfigSecret(kb.Name)
		pp.DeploymentName = kbv1.BackgroundTasksDeployment(kb.Name)
		return pp, nil
	}

	poolCfg, err := primaryPoolConfig.WithOverlay(overlay)
	if err != nil {
		return poolParameters{}, err
	}

	pp.Config = poolCfg
	pp.SecretName = kbv1.BackgroundTasksConfigSecret(kb.Name)
	pp.DeploymentName = kbv1.BackgroundTasksDeployment(kb.Name)
	return pp, nil
}

func reconcileService(ctx context.Context, client k8s.Client, kb *kbv1.Kibana, meta metadata.Metadata) (*corev1.Service, error) {
	svcSelector, err := serviceSelector(ctx, client, kb)
	if err != nil {
		return nil, err
	}

	expectedService := NewService(*kb, meta, svcSelector)

	var existing corev1.Service
	err = client.Get(ctx, k8s.ExtractNamespacedName(expectedService), &existing)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	if err == nil && common.IsOrchestrationPaused(kb) && !maps.Equal(existing.Spec.Selector, expectedService.Spec.Selector) {
		ulog.FromContext(ctx).Info(
			"Service selector change detected but orchestration is paused; skipping Service reconciliation",
			"namespace", kb.Namespace, "kibana_name", kb.Name, "service", expectedService.Name,
		)
		return &existing, nil
	}

	return common.ReconcileService(ctx, client, expectedService, kb)
}

type poolDeploymentResult string

const (
	Requeue poolDeploymentResult = "Requeue"
	Defer   poolDeploymentResult = "Defer"
)

func (d *driver) reconcilePoolDeployment(
	ctx context.Context,
	kb *kbv1.Kibana,
	poolParams poolParameters,
	podAnnotations map[string]string,
	meta metadata.Metadata,
	reconciledSecrets map[string]struct{},
) (poolDeploymentResult, *appsv1.Deployment, error) {
	logger := ulog.FromContext(ctx)

	// Detect immutable selector transition.
	// Kubernetes rejects selector updates as immutable, so we delete the stale deployment and requeue;
	// the next pass recreates it with the correct selector.
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
	if existingSelector != nil && !umaps.IsSubset(expectedSelector, existingSelector) { // selector change is required
		if common.IsOrchestrationPaused(kb) {
			logger.Info(
				"Deployment selector mismatch detected but orchestration is paused; deferring deletion",
				"deployment", existingDeployment.Name,
			)
			return Defer, nil, nil
		}

		logger.Info(
			"Deployment selector mismatch; deleting to allow recreation with updated selector",
			"deployment", existingDeployment.Name,
			"existing_selector", existingSelector,
			"expected_selector", expectedSelector,
		)
		if delErr := k8s.DeleteResourceIfExists(ctx, d.client, &existingDeployment); delErr != nil {
			return "", nil, delErr
		}

		k8s.EmitEvent(d.recorder, kb, corev1.EventTypeNormal, events.EventReasonUpgraded, events.EventActionDeploymentReconciliation,
			fmt.Sprintf("Deleted Deployment %s to apply updated selector", existingDeployment.Name))

		return Requeue, nil, nil
	}

	// reconcile pool secret, unless it was already written earlier in this reconciliation pass
	if _, exists := reconciledSecrets[poolParams.SecretName]; !exists {
		err := ReconcileConfigSecret(ctx, d.client, *kb, poolParams.Config, poolParams.SecretName, meta)
		if err != nil {
			return "", nil, err
		}
		reconciledSecrets[poolParams.SecretName] = struct{}{}
	}

	// Determine replica count for this pool.
	replicas, oldReplicas := deploymentReplicas(ctx, d.client, kb, poolParams, &existingDeployment)

	dpParams, err := d.deploymentParams(
		ctx,
		kb,
		poolParams.Role,
		poolParams.SecretName,
		poolParams.DeploymentName,
		replicas,
		oldReplicas,
		podAnnotations,
		poolParams.BasePath,
		poolParams.OperatorParameters.SetDefaultSecurityContext,
		poolParams.OperatorParameters.OperatorNamespace,
		meta,
		expectedSelector,
	)
	if err != nil {
		return "", nil, err
	}

	expectedDp := deployment.New(dpParams)
	reconciledDp, err := common.ReconcilePauseAware(ctx, d.client, d.recorder, expectedDp, kb, deployment.Reconcile)
	if err != nil {
		return "", nil, err
	}

	return "", &reconciledDp, nil
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

// serviceSelector fetches the existing Kibana HTTP Service (if any) and returns the label
// selector to use for it. It applies the same ECK upgrade-path exception as deploymentSelector:
// when the existing Service has no role label yet and split mode is inactive, the role label
// is omitted so that the Service is not needlessly updated on every reconcile during an
// operator upgrade. Returns the full expected selector (including role=primary) when the Service
// does not exist yet.
func serviceSelector(ctx context.Context, client k8s.Client, kb *kbv1.Kibana) (map[string]string, error) {
	var existingSvc corev1.Service
	if err := client.Get(ctx, types.NamespacedName{Namespace: kb.Namespace, Name: kbv1.HTTPService(kb.Name)}, &existingSvc); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		// kblabel.UIRole is used directly: the HTTP service is only created for the UI role
		// and only ever targets the primary pool.
		return deploymentSelector(kb, kblabel.UIRole, nil), nil
	}
	return deploymentSelector(kb, kblabel.UIRole, existingSvc.Spec.Selector), nil
}

func deploymentReplicas(ctx context.Context, client k8s.Client, kb *kbv1.Kibana, pm poolParameters, existingDeployment *appsv1.Deployment) (newValue *int32, oldValue *int32) {
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

	getBackgroundReplicas := func() *int32 {
		if kb.Spec.BackgroundTasks.Count != nil {
			return kb.Spec.BackgroundTasks.Count
		}

		if existingDeployment != nil && existingDeployment.Spec.Replicas != nil {
			return existingDeployment.Spec.Replicas // get the HPA decided value
		}

		return replicasFromAnnotation() // restore from annotation
	}

	switch {
	case pm.ScaleToZero:
		newValue = new(int32(0)) // drain both pools before starting the new version

		// Return the old value if the deployment is background tasks. It will be stored in annotation.
		if pm.Role.IsBackgroundTasks() {
			oldValue = getBackgroundReplicas()
			if oldValue == nil {
				oldValue = new(int32(1))
			}
		}

	case kb.BackgroundTasksEnabled() && pm.Role.IsBackgroundTasks():
		newValue = getBackgroundReplicas()

		// Delete the annotation since it is no longer needed.
		if err := k8s.PatchAnnotations(ctx, client, existingDeployment, nil, replicasAnnotationName); err != nil {
			// keep the annotation for the next reconciliation; the value read remains valid
			logger.Error(err, "failed to remove replicas annotation", "annotation", replicasAnnotationName, "deployment", existingDeployment.Name)
		}

	default:
		newValue = new(kb.Spec.Count)
	}

	return newValue, oldValue
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

func buildStatus(state *State, kb *kbv1.Kibana, role kblabel.Role, poolStatus commonv1.DeploymentStatus, aggregateHealth commonv1.DeploymentHealth, aggregateAvailable, aggregateCount int32) {
	if kb.BackgroundTasksEnabled() && state.Kibana.Status.Pools == nil {
		state.Kibana.Status.Pools = &kbv1.KibanaPoolsStatuses{}
	}

	switch {
	case role.IsSinglePool():
		state.Kibana.Status.DeploymentStatus = poolStatus
	case role.IsPrimary():
		state.Kibana.Status.DeploymentStatus = poolStatus
		state.Kibana.Status.Pools.Primary = &kbv1.KibanaPoolStatus{
			Selector:       poolStatus.Selector,
			Count:          poolStatus.Count,
			AvailableNodes: poolStatus.AvailableNodes,
			Health:         poolStatus.Health,
		}
	case role.IsBackgroundTasks():
		state.Kibana.Status.Pools.BackgroundTasks = &kbv1.KibanaPoolStatus{
			Selector:       poolStatus.Selector,
			Count:          poolStatus.Count,
			AvailableNodes: poolStatus.AvailableNodes,
			Health:         poolStatus.Health,
		}
	}

	// replace with aggregate in top level
	if !role.IsSinglePool() {
		state.Kibana.Status.DeploymentStatus.Health = aggregateHealth
		state.Kibana.Status.DeploymentStatus.AvailableNodes = aggregateAvailable
		state.Kibana.Status.DeploymentStatus.Count = aggregateCount
		state.Kibana.Status.DeploymentStatus.Selector = metav1.FormatLabelSelector(&metav1.LabelSelector{
			MatchLabels: kb.GetIdentityLabels(), // without the role label
		})
	} else {
		// Single pool: clear pools status.
		state.Kibana.Status.Pools = nil
	}
}
