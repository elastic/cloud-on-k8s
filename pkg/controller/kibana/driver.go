// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"context"
	"crypto/sha256"
	"fmt"

	pkgerrors "github.com/pkg/errors"
	"go.elastic.co/apm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/labels"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
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
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/config"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/es"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	kbname "github.com/elastic/cloud-on-k8s/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/pod"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// initContainersParameters is used to generate the init container that will load the secure settings into a keystore
var initContainersParameters = keystore.InitContainerParameters{
	KeystoreCreateCommand:         "/usr/share/kibana/bin/kibana-keystore create",
	KeystoreAddCommand:            `/usr/share/kibana/bin/kibana-keystore add "$key" --stdin < "$filename"`,
	SecureSettingsVolumeMountPath: keystore.SecureSettingsVolumeMountPath,
	DataVolumePath:                volume.DataVolumeMountPath,
}

// minSupportedVersion is the minimum version of Kibana supported by ECK. Currently this is set to version 6.8.0.
var minSupportedVersion = version.From(6, 8, 0)

type driver struct {
	client         k8s.Client
	dynamicWatches watches.DynamicWatches
	recorder       record.EventRecorder
	version        version.Version
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

// getStrategyType decides which deployment strategy (RollingUpdate or Recreate) to use based on whether the version
// upgrade is in progress. Kibana does not support a smooth rolling upgrade from one version to another:
// running multiple versions simultaneously may lead to concurrency bugs and data corruption.
func (d *driver) getStrategyType(kb *kbv1.Kibana) (appsv1.DeploymentStrategyType, error) {
	var pods corev1.PodList
	var labels client.MatchingLabels = map[string]string{label.KibanaNameLabelName: kb.Name}
	if err := d.client.List(&pods, client.InNamespace(kb.Namespace), labels); err != nil {
		return "", err
	}

	for _, pod := range pods.Items {
		ver, ok := pod.Labels[label.KibanaVersionLabelName]
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
	// setup a keystore with secure settings in an init container, if specified by the user
	keystoreResources, err := keystore.NewResources(
		d,
		kb,
		kbname.KBNamer,
		label.NewLabels(kb.Name),
		initContainersParameters,
	)
	if err != nil {
		return deployment.Params{}, err
	}

	kibanaPodSpec := pod.NewPodTemplateSpec(*kb, keystoreResources)

	// Build a checksum of the configuration, which we can use to cause the Deployment to roll Kibana
	// instances in case of any change in the CA file, secure settings or credentials contents.
	// This is done because Kibana does not support updating those without restarting the process.
	configChecksum := sha256.New224()
	if keystoreResources != nil {
		_, _ = configChecksum.Write([]byte(keystoreResources.Version))
	}
	// we need to deref the secret here (if any) to include it in the checksum otherwise Kibana will not be rolled on contents changes
	if kb.AssociationConf().AuthIsConfigured() {
		esAuthKey := types.NamespacedName{Name: kb.AssociationConf().GetAuthSecretName(), Namespace: kb.Namespace}
		esAuthSecret := corev1.Secret{}
		if err := d.client.Get(esAuthKey, &esAuthSecret); err != nil {
			return deployment.Params{}, err
		}
		_, _ = configChecksum.Write(esAuthSecret.Data[kb.AssociationConf().GetAuthSecretKey()])
	}

	volumes := []commonvolume.SecretVolume{config.SecretVolume(*kb)}

	if kb.AssociationConf().CAIsConfigured() {
		esPublicCAKey := types.NamespacedName{Namespace: kb.Namespace, Name: kb.AssociationConf().GetCASecretName()}
		var esPublicCASecret corev1.Secret
		if err := d.client.Get(esPublicCAKey, &esPublicCASecret); err != nil {
			return deployment.Params{}, err
		}
		if certPem, ok := esPublicCASecret.Data[certificates.CertFileName]; ok {
			_, _ = configChecksum.Write(certPem)
		}

		esCertsVolume := es.CaCertSecretVolume(*kb)
		volumes = append(volumes, esCertsVolume)
		for i := range kibanaPodSpec.Spec.InitContainers {
			kibanaPodSpec.Spec.InitContainers[i].VolumeMounts = append(kibanaPodSpec.Spec.InitContainers[i].VolumeMounts,
				esCertsVolume.VolumeMount())
		}
	}

	if kb.Spec.HTTP.TLS.Enabled() {
		// fetch the secret to calculate the checksum
		var httpCerts corev1.Secret
		err := d.client.Get(types.NamespacedName{
			Namespace: kb.Namespace,
			Name:      certificates.InternalCertsSecretName(kbname.KBNamer, kb.Name),
		}, &httpCerts)
		if err != nil {
			return deployment.Params{}, err
		}
		if httpCert, ok := httpCerts.Data[certificates.CertFileName]; ok {
			_, _ = configChecksum.Write(httpCert)
		}

		httpCertsVolume := certificates.HTTPCertSecretVolume(kbname.KBNamer, kb.Name)
		volumes = append(volumes, httpCertsVolume)
	}

	// attach volumes
	kibanaContainer := pod.GetKibanaContainer(kibanaPodSpec.Spec)
	for _, volume := range volumes {
		kibanaPodSpec.Spec.Volumes = append(kibanaPodSpec.Spec.Volumes, volume.Volume())
		kibanaContainer.VolumeMounts = append(kibanaContainer.VolumeMounts, volume.VolumeMount())
	}

	// get config secret to add its content to the config checksum
	configSecret := corev1.Secret{}
	err = d.client.Get(types.NamespacedName{Name: config.SecretName(*kb), Namespace: kb.Namespace}, &configSecret)
	if err != nil {
		return deployment.Params{}, err
	}
	_, _ = configChecksum.Write(configSecret.Data[config.SettingsFilename])

	// add the checksum to a label for the deployment and its pods (the important bit is that the pod template
	// changes, which will trigger a rolling update)
	kibanaPodSpec.Labels[configChecksumLabel] = fmt.Sprintf("%x", configChecksum.Sum(nil))

	// decide the strategy type
	strategyType, err := d.getStrategyType(kb)
	if err != nil {
		return deployment.Params{}, err
	}

	return deployment.Params{
		Name:            kbname.KBNamer.Suffix(kb.Name),
		Namespace:       kb.Namespace,
		Replicas:        kb.Spec.Count,
		Selector:        label.NewLabels(kb.Name),
		Labels:          label.NewLabels(kb.Name),
		PodTemplateSpec: kibanaPodSpec,
		Strategy:        strategyType,
	}, nil
}

func (d *driver) Reconcile(
	ctx context.Context,
	state *State,
	kb *kbv1.Kibana,
	params operator.Parameters,
) *reconciler.Results {
	results := reconciler.NewResult(ctx)
	if !association.IsConfiguredIfSet(kb, d.recorder) {
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
		Object:                kb,
		TLSOptions:            kb.Spec.HTTP.TLS,
		Namer:                 kbname.KBNamer,
		Labels:                labels.NewLabels(kb.Name),
		Services:              []corev1.Service{*svc},
		CACertRotation:        params.CACertRotation,
		CertRotation:          params.CertRotation,
		GarbageCollectSecrets: true,
	}.ReconcileCAAndHTTPCerts(ctx)
	if results.HasError() {
		return results
	}

	kbSettings, err := config.NewConfigSettings(ctx, d.client, *kb, d.version)
	if err != nil {
		return results.WithError(err)
	}

	err = config.ReconcileConfigSecret(ctx, d.client, *kb, kbSettings, params.OperatorInfo)
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
	state.UpdateKibanaState(reconciledDp)
	return results
}

func newDriver(
	client k8s.Client,
	watches watches.DynamicWatches,
	recorder record.EventRecorder,
	kb *kbv1.Kibana,
) (*driver, error) {
	ver, err := version.Parse(kb.Spec.Version)
	if err != nil {
		k8s.EmitErrorEvent(recorder, err, kb, events.EventReasonValidation, "Invalid version '%s': %v", kb.Spec.Version, err)
		return nil, err
	}

	if !ver.IsSameOrAfter(minSupportedVersion) {
		err := pkgerrors.Errorf("unsupported Kibana version: %s", ver)
		k8s.EmitErrorEvent(recorder, err, kb, events.EventReasonValidation, "Unsupported Kibana version")
		return nil, err
	}

	return &driver{
		client:         client,
		dynamicWatches: watches,
		recorder:       recorder,
		version:        *ver,
	}, nil
}
