// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"fmt"
	"hash/fnv"

	pkgerrors "github.com/pkg/errors"
	"go.elastic.co/apm/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
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

	// reconcile the service (only for the primary pool).
	svc, err := reconcileService(ctx, d.client, kb, meta)
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

	basePath, err := kbv1.GetKibanaBasePath(*kb)
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

	// fetch pod from all pools
	existingPods, err := k8s.PodsMatchingLabels(d.K8sClient(), kb.Namespace, map[string]string{kblabel.KibanaNameLabelName: kb.Name})
	if err != nil {
		return results.WithError(err)
	}

	// When split is active and any pod carries a stale version, enter the stop phase:
	// scale both pools to zero before starting any new-version pods. Without this,
	// the UI Deployment can start new pods while old background-task pods still run,
	// violating Kibana's requirement that all outdated instances stop first.
	scaleToZero := shouldScaleAllPoolsToZeroForUpgrade(kb, existingPods)

	activeRoles := kb.ActiveRoles()

	// Pool statuses are staged locally and committed to state via buildStatus: once after the
	// loop on success, and with whatever entries were gathered so far when a pool fails to
	// reconcile, so the status reflects the pools that were observed this pass.
	poolStatuses := make([]poolStatusEntry, 0, len(activeRoles))
	// Each pool status inherits unknown fields from the previously persisted status, captured
	// before any mutation.
	prevStatus := state.Kibana.Status

	defer func() { buildStatus(state, kb, poolStatuses) }()

	// reconciledSecrets tracks which config secrets have already been written this pass
	// so that pools sharing the base secret don't trigger a redundant write.
	reconciledSecrets := map[string]struct{}{}

	for _, role := range activeRoles {
		poolParams, err := poolParams(params, kb, role, basePath, scaleToZero)
		if err != nil {
			return results.WithError(err)
		}

		// merge with scp.
		poolConfig, err := NewConfigSettings(ctx, d.client, *kb, poolParams.Config, d.version, d.ipFamily, kibanaPolicyCfg.KibanaConfig)
		if err != nil {
			return results.WithError(err)
		}
		poolParams.Config = poolConfig

		deploymentResult, reconciledDp, err := d.reconcilePoolDeployment(
			ctx,
			kb,
			poolParams,
			kibanaPolicyCfg.PodAnnotations,
			meta,
			reconciledSecrets,
		)
		switch {
		case err != nil:
			return results.WithError(err)
		case deploymentResult == deferred:
			return results
		case deploymentResult == requeue:
			return results.WithRequeue()
		}

		currentStatus := prevStatus.Pools.Primary
		if role.IsBackgroundTasks() {
			currentStatus = ptr.Deref(prevStatus.Pools.BackgroundTasks, commonv1.DeploymentStatus{})
		}

		poolStatus, err := common.DeploymentStatus(ctx, currentStatus, *reconciledDp, existingPods, kblabel.KibanaVersionLabelName)
		if err != nil {
			return results.WithError(err)
		}

		poolStatuses = append(poolStatuses, poolStatusEntry{role: role, status: poolStatus})
	}

	if scaleToZero {
		// Both pools are being drained. Requeue so the controller re-checks once pods terminate.
		results = results.WithRequeue()
	}

	return results
}

// garbageCollectBackgroundResources deletes the background tasks Deployment and config Secret
// when spec.backgroundTasks is no longer set.
// Deletion is skipped while orchestration is paused so that disabling the split on a paused
// Kibana does not remove the background Deployment until the user resumes orchestration.
func (d *driver) garbageCollectBackgroundResources(ctx context.Context, kb *kbv1.Kibana) error {
	if common.IsOrchestrationPaused(kb) {
		return nil
	}

	if err := k8s.DeleteResourceIfExists(ctx, d.client, &appsv1.Deployment{
		Namespace: kb.Namespace,
		Name:      kbv1.BackgroundTasksDeployment(kb.Name),
	}); err != nil {
		return err
	}

	return d.garbageCollectBGConfigSecret(ctx, kb)
}

// garbageCollectBGConfigSecret deletes the background tasks config Secret when it is no longer
// needed — either because spec.backgroundTasks was cleared or because spec.backgroundTasks.config
// was cleared (both pools now share the base secret).
func (d *driver) garbageCollectBGConfigSecret(ctx context.Context, kb *kbv1.Kibana) error {
	if common.IsOrchestrationPaused(kb) {
		return nil
	}

	return k8s.DeleteResourceIfExists(ctx, d.client, &corev1.Secret{
		Namespace: kb.Namespace,
		Name:      kbv1.BackgroundTasksConfigSecret(kb.Name),
	})
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
	pp poolParameters,
	replicas *int32,
	replicasAnnotationValue *int32,
	policyAnnotations map[string]string,
	meta metadata.Metadata,
	selector map[string]string,
	overwriteRevisionHistoryLimit *int32,
) (deployment.Params, error) {
	role := pp.Role
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
		pp.OperatorNamespace,
		initContainersParameters,
	)
	if err != nil {
		return deployment.Params{}, err
	}

	volumes, err := d.buildVolumes(kb, pp.SecretName)
	if err != nil {
		return deployment.Params{}, err
	}

	// Pool-specific metadata: add role label to pod labels.
	poolLabels := umaps.Merge(map[string]string{}, meta.Labels)
	poolLabels[kblabel.RoleLabelName] = role.LabelValue
	poolMeta := metadata.Propagate(kb, metadata.Metadata{Labels: poolLabels, Annotations: meta.Annotations})

	kibanaPodSpec, err := NewPodTemplateSpec(ctx, d.client, *kb, role, keystoreResources, volumes, pp.BasePath, pp.SetDefaultSecurityContext, poolMeta, pp.SecretName)
	if err != nil {
		return deployment.Params{}, err
	}

	// When background task isolation is active, set NODE_ROLES on the Kibana container.
	// Kibana reads this env var to determine which roles it runs, taking precedence over
	// kibana.yml.
	if role.Name != "" {
		nodeRolesValue := fmt.Sprintf(`["%s"]`, role.Name)
		for i, c := range kibanaPodSpec.Spec.Containers {
			if c.Name == kbv1.KibanaContainerName {
				kibanaPodSpec.Spec.Containers[i].Env = append(
					[]corev1.EnvVar{{Name: kblabel.NodeRolesEnvVar, Value: nodeRolesValue}},
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
	if err = d.client.Get(ctx, types.NamespacedName{Name: pp.SecretName, Namespace: kb.Namespace}, &configSecret); err != nil {
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

	// add the replicas Deployment's annotation if needed.
	if role.IsBackgroundTasks() && replicasAnnotationValue != nil {
		poolMeta = poolMeta.Merge(metadata.Metadata{Annotations: map[string]string{
			replicasAnnotationName: fmt.Sprintf("%d", *replicasAnnotationValue),
		}})
	}

	if overwriteRevisionHistoryLimit == nil {
		overwriteRevisionHistoryLimit = kb.Spec.RevisionHistoryLimit
	}
	return deployment.Params{
		Name:                 pp.DeploymentName,
		Namespace:            kb.Namespace,
		Replicas:             replicas,
		Selector:             selector,
		Metadata:             poolMeta,
		PodTemplateSpec:      kibanaPodSpec,
		RevisionHistoryLimit: overwriteRevisionHistoryLimit,
		Strategy:             appsv1.DeploymentStrategy{Type: strategyType},
	}, nil
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

func NewService(kb kbv1.Kibana, meta metadata.Metadata, selector map[string]string) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: kb.Spec.HTTP.Service.ObjectMeta,
		Spec:       kb.Spec.HTTP.Service.Spec,
	}

	svc.ObjectMeta.Namespace = kb.Namespace
	svc.ObjectMeta.Name = kbv1.HTTPService(kb.Name)

	ports := []corev1.ServicePort{
		{
			Name:     kb.Spec.HTTP.Protocol(),
			Protocol: corev1.ProtocolTCP,
			Port:     network.HTTPPort,
		},
	}
	return defaults.SetServiceDefaults(&svc, meta, selector, ports)
}
