// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"fmt"
	"hash/fnv"
	"maps"

	pkgerrors "github.com/pkg/errors"
	"go.elastic.co/apm/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	commonassociation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/deployment"
	driver2 "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	commonvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/initcontainer"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/network"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/stackmon"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	umaps "github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

// minSupportedVersion is the minimum version of Kibana supported by ECK. Currently this is set to version 7.0.0.
var minSupportedVersion = version.From(7, 0, 0)

const (
	// nodeRolesEnvVarName is the Kibana container env var that selects which roles a process runs.
	// Its format is a JSON array string, e.g. `["ui"]` or `["background_tasks"]`.
	// This matches the approach used by the serverless kibana-controller; the env var takes
	// precedence over node.roles in kibana.yml so ECK never writes node.roles to the config file.
	nodeRolesEnvVarName = "NODE_ROLES"
)

type driver struct {
	client         k8s.Client
	dynamicWatches watches.DynamicWatches
	recorder       toolsevents.EventRecorder
	version        version.Version
	ipFamily       corev1.IPFamily
}

func (d *driver) DynamicWatches() watches.DynamicWatches {
	return d.dynamicWatches
}

func (d *driver) K8sClient() k8s.Client {
	return d.client
}

func (d *driver) Recorder() toolsevents.EventRecorder {
	return d.recorder
}

var _ driver2.Interface = (*driver)(nil)

func newDriver(
	client k8s.Client,
	watches watches.DynamicWatches,
	recorder toolsevents.EventRecorder,
	kb *kbv1.Kibana,
	ipFamily corev1.IPFamily,
) (*driver, error) {
	ver, err := version.Parse(kb.Spec.Version)
	if err != nil {
		k8s.MaybeEmitErrorEventf(recorder, err, kb, events.EventReasonValidation, events.EventActionValidation, "Invalid version '%s': %v", kb.Spec.Version, err)
		return nil, err
	}

	if !ver.GTE(minSupportedVersion) {
		err := pkgerrors.Errorf("unsupported Kibana version: %s", ver)
		k8s.MaybeEmitErrorEvent(recorder, err, kb, events.EventReasonValidation, events.EventActionVersionCheck, "Unsupported Kibana version")
		return nil, err
	}

	return &driver{
		client:         client,
		dynamicWatches: watches,
		recorder:       recorder,
		version:        ver,
		ipFamily:       ipFamily,
	}, nil
}

func (d *driver) Reconcile(
	ctx context.Context,
	state *State,
	kb *kbv1.Kibana,
	params operator.Parameters,
) *reconciler.Results {
	results := reconciler.NewResult(ctx)
	isEsAssocConfigured, err := association.IsConfiguredIfSet(ctx, kb.EsAssociation(), d.recorder)
	if err != nil {
		return results.WithError(err)
	}
	if !isEsAssocConfigured {
		return results
	}
	isEntAssocConfigured, err := association.IsConfiguredIfSet(ctx, kb.EntAssociation(), d.recorder)
	if err != nil {
		return results.WithError(err)
	}
	if !isEntAssocConfigured {
		return results
	}
	isEPRAssocConfigured, err := association.IsConfiguredIfSet(ctx, kb.EPRAssociation(), d.recorder)
	if err != nil {
		return results.WithError(err)
	}
	if !isEPRAssocConfigured {
		return results
	}

	// metadata to propagate to children
	meta := metadata.Propagate(kb, metadata.Metadata{Labels: kb.GetIdentityLabels()})

	// Service selector: point only at UI-role pods when split mode is active.
	svc, err := common.ReconcileService(ctx, d.client, NewService(*kb, meta), kb)
	if err != nil {
		return results.WithError(err)
	}

	_, results = certificates.Reconciler{
		K8sClient:             d.K8sClient(),
		DynamicWatches:        d.DynamicWatches(),
		Owner:                 kb,
		TLSOptions:            kb.Spec.HTTP.TLS,
		Namer:                 kbv1.KBNamer,
		Metadata:              meta,
		Services:              []corev1.Service{*svc},
		GlobalCA:              params.GlobalCA,
		CACertRotation:        params.CACertRotation,
		CertRotation:          params.CertRotation,
		GarbageCollectSecrets: true,
	}.ReconcileCAAndHTTPCerts(ctx)
	if results.HasError() {
		_, err := results.Aggregate()
		k8s.MaybeEmitErrorEventf(d.Recorder(), err, kb, events.EventReconciliationError, events.EventActionCertificateReconciliation, "Certificate reconciliation error: %v", err)
		return results
	}

	logger := ulog.FromContext(ctx)
	assocAllowed, err := association.AllowVersion(d.version, kb, logger, d.Recorder())
	if err != nil {
		return results.WithError(err)
	}
	if !assocAllowed {
		return results // will eventually retry
	}

	kibanaPolicyCfg, err := getPolicyConfig(ctx, d.client, *kb)
	if err != nil {
		return results.WithError(err)
	}

	// Compute the base config once; per-pool copies apply node.roles on top.
	baseSettings, err := NewConfigSettings(ctx, d.client, *kb, d.version, d.ipFamily, kibanaPolicyCfg.KibanaConfig)
	if err != nil {
		return results.WithError(err)
	}

	basePath, err := GetKibanaBasePath(*kb)
	if err != nil {
		return results.WithError(err)
	}

	if err = stackmon.ReconcileConfigSecrets(ctx, d.client, *kb, basePath, meta); err != nil {
		return results.WithError(err)
	}

	if err = initcontainer.ReconcileScriptsConfigMap(ctx, d.client, *kb, meta); err != nil {
		return results.WithError(err)
	}

	// GC: remove background-tasks resources when the split is no longer configured.
	if !kb.BackgroundTasksEnabled() {
		if err := d.garbageCollectBackgroundResources(ctx, kb); err != nil {
			return results.WithError(err)
		}
	} else if kb.Spec.BackgroundTasks.Config == nil {
		// BG pool is active but has no overlay — both pools share the base secret.
		// GC any orphaned BG config secret left from a previous overlay configuration.
		if err := d.garbageCollectBGConfigSecret(ctx, kb); err != nil {
			return results.WithError(err)
		}
	}

	span, _ := apm.StartSpan(ctx, "reconcile_deployment", tracing.SpanTypeApp)
	defer span.End()

	// Fan-out: reconcile one Deployment per active pool.
	existingPods, err := k8s.PodsMatchingLabels(d.K8sClient(), kb.Namespace, map[string]string{kblabel.KibanaNameLabelName: kb.Name})
	if err != nil {
		return results.WithError(err)
	}

	var uiDeployment appsv1.Deployment
	var aggregateAvailable int32
	var aggregateHealth commonv1.DeploymentHealth = commonv1.GreenHealth

	// reconciledSecrets tracks which config secrets have already been written this pass
	// so that pools sharing the base secret don't trigger a redundant write.
	reconciledSecrets := map[string]bool{}

	for i, role := range kb.ActiveRoles() {
		poolCfg, poolSecretName, poolDeploymentName, err := d.poolParams(kb, role, baseSettings)
		if err != nil {
			return results.WithError(err)
		}

		// D5: detect immutable selector transition.
		// Enabling or disabling spec.backgroundTasks changes the deployment selector
		// (adds/removes the role label). Kubernetes rejects selector updates as immutable,
		// so we delete the stale deployment and requeue; the next pass recreates it with
		// the correct selector.
		expectedSelector := kb.GetPoolIdentityLabels(role)
		var existingForSelector appsv1.Deployment
		if err := d.client.Get(ctx, types.NamespacedName{Namespace: kb.Namespace, Name: poolDeploymentName}, &existingForSelector); err == nil {
			if !maps.Equal(existingForSelector.Spec.Selector.MatchLabels, expectedSelector) {
				logger.Info(
					"Deployment selector mismatch; deleting to allow recreation with updated selector",
					"deployment", poolDeploymentName,
					"existing_selector", existingForSelector.Spec.Selector.MatchLabels,
					"expected_selector", expectedSelector,
				)
				if delErr := d.client.Delete(ctx, &existingForSelector); delErr != nil && !apierrors.IsNotFound(delErr) {
					return results.WithError(delErr)
				}
				k8s.EmitEvent(d.recorder, kb, corev1.EventTypeNormal, events.EventReasonUpgraded, events.EventActionDeploymentReconciliation,
					fmt.Sprintf("Deleted Deployment %s to apply updated selector", poolDeploymentName))
				return results.WithRequeue()
			}
		} else if !apierrors.IsNotFound(err) {
			return results.WithError(err)
		}

		if !reconciledSecrets[poolSecretName] {
			if err = ReconcileConfigSecret(ctx, d.client, *kb, poolCfg, poolSecretName, meta); err != nil {
				return results.WithError(err)
			}
			reconciledSecrets[poolSecretName] = true
		}

		// Determine replica count for this pool.
		var replicas *int32
		if role.Name == kblabel.BackgroundTasksRole.Name && kb.Spec.BackgroundTasks != nil {
			replicas = kb.Spec.BackgroundTasks.Count // nil means HPA-managed
		} else {
			replicas = new(kb.Spec.Count)
		}

		dpParams, err := d.deploymentParams(ctx, kb, role, poolSecretName, poolDeploymentName, replicas, kibanaPolicyCfg.PodAnnotations, basePath, params.SetDefaultSecurityContext, params.OperatorNamespace, meta)
		if err != nil {
			return results.WithError(err)
		}

		expectedDp := deployment.New(dpParams)
		reconciledDp, err := common.ReconcilePauseAware(ctx, d.client, d.recorder, expectedDp, kb, deployment.Reconcile)
		if err != nil {
			return results.WithError(err)
		}

		poolStatus, err := common.DeploymentStatus(ctx, state.Kibana.Status.DeploymentStatus, reconciledDp, existingPods, kblabel.KibanaVersionLabelName)
		if err != nil {
			return results.WithError(err)
		}

		aggregateAvailable += poolStatus.AvailableNodes
		if poolStatus.Health != commonv1.GreenHealth {
			aggregateHealth = poolStatus.Health
		}

		if i == 0 {
			// First pool is always the UI/single pool — it drives the scale sub-resource.
			uiDeployment = reconciledDp
			state.Kibana.Status.DeploymentStatus = poolStatus
		}

		// Populate background tasks pool status.
		if role.Name == kblabel.BackgroundTasksRole.Name {
			bgPoolStatus := &kbv1.KibanaPoolStatus{
				Selector:       poolStatus.Selector,
				Count:          poolStatus.Count,
				AvailableNodes: poolStatus.AvailableNodes,
				Health:         poolStatus.Health,
			}
			state.Kibana.Status.BackgroundTasks = bgPoolStatus
		}
	}

	if kb.BackgroundTasksEnabled() {
		// Aggregate across all pools; UI pool count/selector already set above.
		uiStatus, err := common.DeploymentStatus(ctx, state.Kibana.Status.DeploymentStatus, uiDeployment, existingPods, kblabel.KibanaVersionLabelName)
		if err != nil {
			return results.WithError(err)
		}
		uiStatus.AvailableNodes = aggregateAvailable
		uiStatus.Health = aggregateHealth
		state.Kibana.Status.DeploymentStatus = uiStatus
	} else {
		// Single pool: clear background tasks status.
		state.Kibana.Status.BackgroundTasks = nil
	}

	return results
}

// poolParams returns the per-pool config, secret name, and deployment name for a given role.
// node.roles is NOT injected into kibana.yml — it is set via NODE_ROLES env var on the pod,
// matching the approach used by the serverless kibana-controller.
//
// Both pools share the same config secret unless spec.backgroundTasks.config is set, in which
// case the background tasks pool gets its own secret with the overlay merged on top.
func (d *driver) poolParams(kb *kbv1.Kibana, role kblabel.Role, base CanonicalConfig) (CanonicalConfig, string, string, error) {
	if role.Name == "" || role.Name == kblabel.UIRole.Name {
		// Single all-roles pool or UI pool: use the base config and base names.
		return base, kbv1.ConfigSecret(kb.Name), kbv1.KBNamer.Suffix(kb.Name), nil
	}

	// Background tasks pool: only create a separate secret when there is an overlay.
	overlay := kb.Spec.BackgroundTasks.Config // kb.Spec.BackgroundTasks is non-nil when BG role is active
	if overlay == nil {
		// No overlay — share the base secret; no separate BG secret needed.
		return base, kbv1.ConfigSecret(kb.Name), kbv1.BackgroundTasksDeployment(kb.Name), nil
	}

	poolCfg, err := base.WithPoolOverlay(overlay)
	if err != nil {
		return CanonicalConfig{}, "", "", err
	}
	return poolCfg, kbv1.BackgroundTasksConfigSecret(kb.Name), kbv1.BackgroundTasksDeployment(kb.Name), nil
}

// garbageCollectBackgroundResources deletes the background tasks Deployment and config Secret
// when spec.backgroundTasks is no longer set.
func (d *driver) garbageCollectBackgroundResources(ctx context.Context, kb *kbv1.Kibana) error {
	bgDpName := kbv1.BackgroundTasksDeployment(kb.Name)
	var bgDp appsv1.Deployment
	if err := d.client.Get(ctx, types.NamespacedName{Namespace: kb.Namespace, Name: bgDpName}, &bgDp); err == nil {
		if err := d.client.Delete(ctx, &bgDp); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	return d.garbageCollectBGConfigSecret(ctx, kb)
}

// garbageCollectBGConfigSecret deletes the background tasks config Secret when it is no longer
// needed — either because spec.backgroundTasks was cleared or because spec.backgroundTasks.config
// was cleared (both pools now share the base secret).
func (d *driver) garbageCollectBGConfigSecret(ctx context.Context, kb *kbv1.Kibana) error {
	bgSecretName := kbv1.BackgroundTasksConfigSecret(kb.Name)
	var bgSecret corev1.Secret
	if err := d.client.Get(ctx, types.NamespacedName{Namespace: kb.Namespace, Name: bgSecretName}, &bgSecret); err == nil {
		if err := d.client.Delete(ctx, &bgSecret); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// getStrategyType decides which deployment strategy (RollingUpdate or Recreate) to use based on whether the version
// upgrade is in progress. Kibana does not support a smooth rolling upgrade from one version to another:
// running multiple versions simultaneously may lead to concurrency bugs and data corruption.
func (d *driver) getStrategyType(kb *kbv1.Kibana) (appsv1.DeploymentStrategyType, error) {
	var pods corev1.PodList
	var labels client.MatchingLabels = map[string]string{kblabel.KibanaNameLabelName: kb.Name}
	if err := d.client.List(context.Background(), &pods, client.InNamespace(kb.Namespace), labels); err != nil {
		return "", err
	}

	for _, pod := range pods.Items {
		ver, ok := pod.Labels[kblabel.KibanaVersionLabelName]
		// if label is missing we assume that the last reconciliation was done by previous version of the operator
		// to be safe, we assume the Kibana version has changed when operator was offline and use Recreate,
		// otherwise we may run into data corruption/data loss.
		if !ok || ver != kb.Spec.Version {
			return appsv1.RecreateDeploymentStrategyType, nil
		}
	}

	return appsv1.RollingUpdateDeploymentStrategyType, nil
}

func (d *driver) deploymentParams(
	ctx context.Context,
	kb *kbv1.Kibana,
	role kblabel.Role,
	configSecretName string,
	deploymentName string,
	replicas *int32,
	policyAnnotations map[string]string,
	basePath string,
	setDefaultSecurityContext bool,
	operatorNamespace string,
	meta metadata.Metadata,
) (deployment.Params, error) {
	initContainersParameters, err := initcontainer.NewInitContainersParameters(kb)
	if err != nil {
		return deployment.Params{}, err
	}
	// setup a keystore with secure settings in an init container, if specified by the user
	keystoreResources, err := keystore.ReconcileResources(
		ctx,
		d,
		kb,
		kbv1.KBNamer,
		meta,
		operatorNamespace,
		initContainersParameters,
	)
	if err != nil {
		return deployment.Params{}, err
	}

	volumes, err := d.buildVolumes(kb, configSecretName)
	if err != nil {
		return deployment.Params{}, err
	}

	// For the background tasks pool, use the pool-specific pod template and resources.
	kbForPool := *kb
	if role.Name == kblabel.BackgroundTasksRole.Name && kb.Spec.BackgroundTasks != nil {
		kbForPool.Spec.PodTemplate = mergePoolPodTemplate(kb.Spec.PodTemplate, kb.Spec.BackgroundTasks.PodTemplate)
		if !kb.Spec.BackgroundTasks.Resources.IsEmpty() {
			kbForPool.Spec.Resources = kb.Spec.BackgroundTasks.Resources
		}
	}

	// Pool-specific metadata: add role label to pod labels.
	poolMeta := meta
	if role.LabelName != "" {
		poolLabels := umaps.Merge(map[string]string{}, meta.Labels)
		poolLabels[role.LabelName] = kblabel.RoleLabelValue
		poolMeta = metadata.Propagate(kb, metadata.Metadata{Labels: poolLabels, Annotations: meta.Annotations})
	}

	kibanaPodSpec, err := NewPodTemplateSpec(ctx, d.client, kbForPool, keystoreResources, volumes, basePath, setDefaultSecurityContext, poolMeta, configSecretName)
	if err != nil {
		return deployment.Params{}, err
	}

	// When background task isolation is active, set NODE_ROLES on the Kibana container.
	// Kibana reads this env var to determine which roles it runs, taking precedence over
	// kibana.yml. This matches how the serverless kibana-controller assigns roles.
	if role.Name != "" {
		nodeRolesValue := fmt.Sprintf(`["%s"]`, role.Name)
		for i, c := range kibanaPodSpec.Spec.Containers {
			if c.Name == kbv1.KibanaContainerName {
				kibanaPodSpec.Spec.Containers[i].Env = append(
					[]corev1.EnvVar{{Name: nodeRolesEnvVarName, Value: nodeRolesValue}},
					kibanaPodSpec.Spec.Containers[i].Env...,
				)
				break
			}
		}
	}

	// Build a checksum of the configuration, which we can use to cause the Deployment to roll Kibana
	// instances in case of any change in the CA file, secure settings or credentials contents.
	// This is done because Kibana does not support updating those without restarting the process.
	configHash := fnv.New32a()
	if keystoreResources != nil {
		_, _ = configHash.Write([]byte(keystoreResources.Hash))
	}

	// we need to deref the secret here to include it in the checksum otherwise Kibana will not be rolled on contents changes
	if err := commonassociation.WriteAssocsToConfigHash(d.client, kb.GetAssociations(), configHash); err != nil {
		return deployment.Params{}, err
	}

	if kb.Spec.HTTP.TLS.Enabled() {
		// fetch the secret to calculate the checksum
		var httpCerts corev1.Secret
		err := d.client.Get(ctx, types.NamespacedName{
			Namespace: kb.Namespace,
			Name:      certificates.InternalCertsSecretName(kbv1.KBNamer, kb.Name),
		}, &httpCerts)
		if err != nil {
			return deployment.Params{}, err
		}
		if httpCert, ok := httpCerts.Data[certificates.CertFileName]; ok {
			_, _ = configHash.Write(httpCert)
		}
	}

	// Hash the pool-specific config secret so changes roll this deployment.
	var configSecret corev1.Secret
	if err = d.client.Get(ctx, types.NamespacedName{Name: configSecretName, Namespace: kb.Namespace}, &configSecret); err != nil {
		return deployment.Params{}, err
	}
	_, _ = configHash.Write(configSecret.Data[SettingsFilename])

	// add the checksum to an annotation for the deployment and its pods (the important bit is that the pod template
	// changes, which will trigger a rolling update)
	kibanaPodSpec.Annotations[configHashAnnotationName] = fmt.Sprint(configHash.Sum32())

	// add additional annotations related to the StackConfigPolicy
	kibanaPodSpec.Annotations = umaps.Merge(kibanaPodSpec.Annotations, policyAnnotations)

	// decide the strategy type
	strategyType, err := d.getStrategyType(kb)
	if err != nil {
		return deployment.Params{}, err
	}

	selector := kb.GetPoolIdentityLabels(role)

	return deployment.Params{
		Name:                 deploymentName,
		Namespace:            kb.Namespace,
		Replicas:             replicas,
		Selector:             selector,
		Metadata:             poolMeta,
		PodTemplateSpec:      kibanaPodSpec,
		RevisionHistoryLimit: kb.Spec.RevisionHistoryLimit,
		Strategy:             appsv1.DeploymentStrategy{Type: strategyType},
	}, nil
}

// mergePoolPodTemplate overlays the pool-specific template on top of the base template.
// Each field is merged independently: pool wins when it carries a non-zero value, base is kept
// otherwise. Metadata maps (labels, annotations) are merged rather than replaced.
// Containers and init containers are merged by name: a pool container that matches a base
// container by name overlays individual fields; unmatched pool containers are appended.
func mergePoolPodTemplate(base, pool corev1.PodTemplateSpec) corev1.PodTemplateSpec {
	merged := *base.DeepCopy()

	// ObjectMeta: merge maps so pool additions don't wipe base entries.
	if len(pool.Labels) > 0 {
		merged.Labels = maps.Clone(merged.Labels)
		maps.Copy(merged.Labels, pool.Labels)
	}
	if len(pool.Annotations) > 0 {
		merged.Annotations = maps.Clone(merged.Annotations)
		maps.Copy(merged.Annotations, pool.Annotations)
	}

	// Scheduling fields: pool wins when explicitly set.
	if pool.Spec.NodeSelector != nil {
		merged.Spec.NodeSelector = pool.Spec.NodeSelector
	}
	if pool.Spec.Affinity != nil {
		merged.Spec.Affinity = pool.Spec.Affinity
	}
	if len(pool.Spec.Tolerations) > 0 {
		merged.Spec.Tolerations = pool.Spec.Tolerations
	}
	if len(pool.Spec.TopologySpreadConstraints) > 0 {
		merged.Spec.TopologySpreadConstraints = pool.Spec.TopologySpreadConstraints
	}
	if pool.Spec.PriorityClassName != "" {
		merged.Spec.PriorityClassName = pool.Spec.PriorityClassName
	}
	if pool.Spec.ServiceAccountName != "" {
		merged.Spec.ServiceAccountName = pool.Spec.ServiceAccountName
	}
	if pool.Spec.SecurityContext != nil {
		merged.Spec.SecurityContext = pool.Spec.SecurityContext
	}

	// Containers: merge by name; unmatched pool containers are appended.
	if len(pool.Spec.Containers) > 0 {
		merged.Spec.Containers = mergeContainerList(merged.Spec.Containers, pool.Spec.Containers)
	}
	if len(pool.Spec.InitContainers) > 0 {
		merged.Spec.InitContainers = mergeContainerList(merged.Spec.InitContainers, pool.Spec.InitContainers)
	}

	// Volumes: pool additions win for volumes with the same name; novel volumes are appended.
	if len(pool.Spec.Volumes) > 0 {
		merged.Spec.Volumes = mergeVolumeList(merged.Spec.Volumes, pool.Spec.Volumes)
	}

	return merged
}

// mergeContainerList merges pool containers into base by name.
// A pool container that matches a base container by name replaces it entirely.
// Pool containers with no matching base entry are appended.
func mergeContainerList(base, pool []corev1.Container) []corev1.Container {
	result := make([]corev1.Container, len(base))
	copy(result, base)
	for _, pc := range pool {
		found := false
		for i, bc := range result {
			if bc.Name == pc.Name {
				result[i] = pc
				found = true
				break
			}
		}
		if !found {
			result = append(result, pc)
		}
	}
	return result
}

// mergeVolumeList merges pool volumes into base by name.
// A pool volume that matches a base volume by name replaces it.
// Novel pool volumes are appended.
func mergeVolumeList(base, pool []corev1.Volume) []corev1.Volume {
	result := make([]corev1.Volume, len(base))
	copy(result, base)
	for _, pv := range pool {
		found := false
		for i, bv := range result {
			if bv.Name == pv.Name {
				result[i] = pv
				found = true
				break
			}
		}
		if !found {
			result = append(result, pv)
		}
	}
	return result
}

func (d *driver) buildVolumes(kb *kbv1.Kibana, configSecretName string) ([]commonvolume.VolumeLike, error) {
	volumes := []commonvolume.VolumeLike{DataVolume, initcontainer.ConfigSharedVolume, initcontainer.ConfigVolume(configSecretName)}

	esAssocConf, err := kb.EsAssociation().AssociationConf()
	if err != nil {
		return nil, err
	}
	if esAssocConf.CAIsConfigured() {
		esCertsVolume := esCaCertSecretVolume(*esAssocConf)
		volumes = append(volumes, esCertsVolume)
	}
	if esAssocConf.ClientCertIsConfigured() {
		esClientCertVolume := esClientCertSecretVolume(*esAssocConf)
		volumes = append(volumes, esClientCertVolume)
	}

	entAssocConf, err := kb.EntAssociation().AssociationConf()
	if err != nil {
		return nil, err
	}
	if entAssocConf.CAIsConfigured() {
		entCertsVolume := entCaCertSecretVolume(*entAssocConf)
		volumes = append(volumes, entCertsVolume)
	}

	eprAssocConf, err := kb.EPRAssociation().AssociationConf()
	if err != nil {
		return nil, err
	}
	if eprAssocConf.CAIsConfigured() {
		eprCertsVolume := eprCaCertSecretVolume(*eprAssocConf)
		volumes = append(volumes, eprCertsVolume)
	}

	if kb.Spec.HTTP.TLS.Enabled() {
		httpCertsVolume := certificates.HTTPCertSecretVolume(kbv1.KBNamer, kb.Name)
		volumes = append(volumes, httpCertsVolume)
	}
	return volumes, nil
}

func NewService(kb kbv1.Kibana, meta metadata.Metadata) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: kb.Spec.HTTP.Service.ObjectMeta,
		Spec:       kb.Spec.HTTP.Service.Spec,
	}

	svc.ObjectMeta.Namespace = kb.Namespace
	svc.ObjectMeta.Name = kbv1.HTTPService(kb.Name)

	// When background task isolation is active, the service must select only UI-role pods
	// so that HTTP traffic never lands on background_tasks-only nodes.
	selector := kb.GetPoolIdentityLabels(kblabel.UIRole)
	ports := []corev1.ServicePort{
		{
			Name:     kb.Spec.HTTP.Protocol(),
			Protocol: corev1.ProtocolTCP,
			Port:     network.HTTPPort,
		},
	}
	return defaults.SetServiceDefaults(&svc, meta, selector, ports)
}
