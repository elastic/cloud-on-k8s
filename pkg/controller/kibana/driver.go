// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"fmt"
	"hash/fnv"

	pkgerrors "github.com/pkg/errors"
	"go.elastic.co/apm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	commonassociation "github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	driver2 "github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	commonvolume "github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/network"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/stackmon"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// minSupportedVersion is the minimum version of Kibana supported by ECK. Currently this is set to version 6.8.0.
var minSupportedVersion = version.From(6, 8, 0)

type driver struct {
	client         k8s.Client
	dynamicWatches watches.DynamicWatches
	recorder       record.EventRecorder
	version        version.Version
	ipFamily       corev1.IPFamily
}

func (d *driver) DynamicWatches() watches.DynamicWatches {
	return d.dynamicWatches
}

func (d *driver) K8sClient() k8s.Client {
	return d.client
}

func (d *driver) Recorder() record.EventRecorder {
	return d.recorder
}

var _ driver2.Interface = &driver{}

func newDriver(
	client k8s.Client,
	watches watches.DynamicWatches,
	recorder record.EventRecorder,
	kb *kbv1.Kibana,
	ipFamily corev1.IPFamily,
) (*driver, error) {
	ver, err := version.Parse(kb.Spec.Version)
	if err != nil {
		k8s.EmitErrorEvent(recorder, err, kb, events.EventReasonValidation, "Invalid version '%s': %v", kb.Spec.Version, err)
		return nil, err
	}

	if !ver.GTE(minSupportedVersion) {
		err := pkgerrors.Errorf("unsupported Kibana version: %s", ver)
		k8s.EmitErrorEvent(recorder, err, kb, events.EventReasonValidation, "Unsupported Kibana version")
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
	isEsAssocConfigured, err := association.IsConfiguredIfSet(kb.EsAssociation(), d.recorder)
	if err != nil {
		return results.WithError(err)
	}
	if !isEsAssocConfigured {
		return results
	}
	isEntAssocConfigured, err := association.IsConfiguredIfSet(kb.EntAssociation(), d.recorder)
	if err != nil {
		return results.WithError(err)
	}
	if !isEntAssocConfigured {
		return results
	}

	svc, err := common.ReconcileService(ctx, d.client, NewService(*kb), kb)
	if err != nil {
		// TODO: consider updating some status here?
		return results.WithError(err)
	}

	_, results = certificates.Reconciler{
		K8sClient:             d.K8sClient(),
		DynamicWatches:        d.DynamicWatches(),
		Owner:                 kb,
		TLSOptions:            kb.Spec.HTTP.TLS,
		Namer:                 kbv1.KBNamer,
		Labels:                NewLabels(kb.Name),
		Services:              []corev1.Service{*svc},
		CACertRotation:        params.CACertRotation,
		CertRotation:          params.CertRotation,
		GarbageCollectSecrets: true,
	}.ReconcileCAAndHTTPCerts(ctx)
	if results.HasError() {
		_, err := results.Aggregate()
		k8s.EmitErrorEvent(d.Recorder(), err, kb, events.EventReconciliationError, "Certificate reconciliation error: %v", err)
		return results
	}

	logger := log.WithValues("namespace", kb.Namespace, "kb_name", kb.Name)
	assocAllowed, err := association.AllowVersion(d.version, kb, logger, d.Recorder())
	if err != nil {
		return results.WithError(err)
	}
	if !assocAllowed {
		return results // will eventually retry
	}

	kbSettings, err := NewConfigSettings(ctx, d.client, *kb, d.version, d.ipFamily)
	if err != nil {
		return results.WithError(err)
	}

	err = ReconcileConfigSecret(ctx, d.client, *kb, kbSettings)
	if err != nil {
		return results.WithError(err)
	}

	err = stackmon.ReconcileConfigSecrets(d.client, *kb)
	if err != nil {
		return results.WithError(err)
	}

	span, _ := apm.StartSpan(ctx, "reconcile_deployment", tracing.SpanTypeApp)
	defer span.End()

	deploymentParams, err := d.deploymentParams(kb)
	if err != nil {
		return results.WithError(err)
	}

	expectedDp := deployment.New(deploymentParams)
	reconciledDp, err := deployment.Reconcile(d.client, expectedDp, kb)
	if err != nil {
		return results.WithError(err)
	}

	existingPods, err := k8s.PodsMatchingLabels(d.K8sClient(), kb.Namespace, map[string]string{KibanaNameLabelName: kb.Name})
	if err != nil {
		return results.WithError(err)
	}
	deploymentStatus, err := common.DeploymentStatus(state.Kibana.Status.DeploymentStatus, reconciledDp, existingPods, KibanaVersionLabelName)
	if err != nil {
		return results.WithError(err)
	}
	state.Kibana.Status.DeploymentStatus = deploymentStatus

	return results
}

// getStrategyType decides which deployment strategy (RollingUpdate or Recreate) to use based on whether the version
// upgrade is in progress. Kibana does not support a smooth rolling upgrade from one version to another:
// running multiple versions simultaneously may lead to concurrency bugs and data corruption.
func (d *driver) getStrategyType(kb *kbv1.Kibana) (appsv1.DeploymentStrategyType, error) {
	var pods corev1.PodList
	var labels client.MatchingLabels = map[string]string{KibanaNameLabelName: kb.Name}
	if err := d.client.List(context.Background(), &pods, client.InNamespace(kb.Namespace), labels); err != nil {
		return "", err
	}

	for _, pod := range pods.Items {
		ver, ok := pod.Labels[KibanaVersionLabelName]
		// if label is missing we assume that the last reconciliation was done by previous version of the operator
		// to be safe, we assume the Kibana version has changed when operator was offline and use Recreate,
		// otherwise we may run into data corruption/data loss.
		if !ok || ver != kb.Spec.Version {
			return appsv1.RecreateDeploymentStrategyType, nil
		}
	}

	return appsv1.RollingUpdateDeploymentStrategyType, nil
}

func (d *driver) deploymentParams(kb *kbv1.Kibana) (deployment.Params, error) {
	initContainersParameters, err := newInitContainersParameters(kb)
	if err != nil {
		return deployment.Params{}, err
	}
	// setup a keystore with secure settings in an init container, if specified by the user
	keystoreResources, err := keystore.ReconcileResources(
		d,
		kb,
		kbv1.KBNamer,
		NewLabels(kb.Name),
		initContainersParameters,
	)
	if err != nil {
		return deployment.Params{}, err
	}

	volumes, err := d.buildVolumes(kb)
	if err != nil {
		return deployment.Params{}, err
	}
	kibanaPodSpec, err := NewPodTemplateSpec(d.client, *kb, keystoreResources, volumes)
	if err != nil {
		return deployment.Params{}, err
	}

	// Build a checksum of the configuration, which we can use to cause the Deployment to roll Kibana
	// instances in case of any change in the CA file, secure settings or credentials contents.
	// This is done because Kibana does not support updating those without restarting the process.
	configHash := fnv.New32a()
	if keystoreResources != nil {
		_, _ = configHash.Write([]byte(keystoreResources.Version))
	}

	// we need to deref the secret here to include it in the checksum otherwise Kibana will not be rolled on contents changes
	if err := commonassociation.WriteAssocsToConfigHash(d.client, kb.GetAssociations(), configHash); err != nil {
		return deployment.Params{}, err
	}

	if kb.Spec.HTTP.TLS.Enabled() {
		// fetch the secret to calculate the checksum
		var httpCerts corev1.Secret
		err := d.client.Get(context.Background(), types.NamespacedName{
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

	// get config secret to add its content to the config checksum
	configSecret := corev1.Secret{}
	err = d.client.Get(context.Background(), types.NamespacedName{Name: SecretName(*kb), Namespace: kb.Namespace}, &configSecret)
	if err != nil {
		return deployment.Params{}, err
	}
	_, _ = configHash.Write(configSecret.Data[SettingsFilename])

	// add the checksum to an annotation for the deployment and its pods (the important bit is that the pod template
	// changes, which will trigger a rolling update)
	kibanaPodSpec.Annotations[configHashAnnotationName] = fmt.Sprint(configHash.Sum32())

	// decide the strategy type
	strategyType, err := d.getStrategyType(kb)
	if err != nil {
		return deployment.Params{}, err
	}

	return deployment.Params{
		Name:            kbv1.KBNamer.Suffix(kb.Name),
		Namespace:       kb.Namespace,
		Replicas:        kb.Spec.Count,
		Selector:        NewLabels(kb.Name),
		Labels:          NewLabels(kb.Name),
		PodTemplateSpec: kibanaPodSpec,
		Strategy:        appsv1.DeploymentStrategy{Type: strategyType},
	}, nil
}

func (d *driver) buildVolumes(kb *kbv1.Kibana) ([]commonvolume.VolumeLike, error) {
	volumes := []commonvolume.VolumeLike{DataVolume, ConfigSharedVolume, ConfigVolume(*kb)}

	esAssocConf, err := kb.EsAssociation().AssociationConf()
	if err != nil {
		return nil, err
	}
	if esAssocConf.CAIsConfigured() {
		esCertsVolume := esCaCertSecretVolume(*esAssocConf)
		volumes = append(volumes, esCertsVolume)
	}

	entAssocConf, err := kb.EntAssociation().AssociationConf()
	if err != nil {
		return nil, err
	}
	if entAssocConf.CAIsConfigured() {
		entCertsVolume := entCaCertSecretVolume(*entAssocConf)
		volumes = append(volumes, entCertsVolume)
	}

	if kb.Spec.HTTP.TLS.Enabled() {
		httpCertsVolume := certificates.HTTPCertSecretVolume(kbv1.KBNamer, kb.Name)
		volumes = append(volumes, httpCertsVolume)
	}
	return volumes, nil
}

func NewService(kb kbv1.Kibana) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: kb.Spec.HTTP.Service.ObjectMeta,
		Spec:       kb.Spec.HTTP.Service.Spec,
	}

	svc.ObjectMeta.Namespace = kb.Namespace
	svc.ObjectMeta.Name = kbv1.HTTPService(kb.Name)

	labels := NewLabels(kb.Name)
	ports := []corev1.ServicePort{
		{
			Name:     kb.Spec.HTTP.Protocol(),
			Protocol: corev1.ProtocolTCP,
			Port:     network.HTTPPort,
		},
	}
	return defaults.SetServiceDefaults(&svc, labels, labels, ports)
}
