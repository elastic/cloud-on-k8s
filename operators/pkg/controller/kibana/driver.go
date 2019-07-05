// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"crypto/sha256"
	"fmt"

	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/securesettings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	kbcerts "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/config"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/es"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/label"
	kbname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/version/version6"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/version/version7"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

type driver struct {
	client          k8s.Client
	scheme          *runtime.Scheme
	settingsFactory func(kb kbtype.Kibana) map[string]interface{}
	dynamicWatches  watches.DynamicWatches
	recorder        record.EventRecorder
}

func secretWatchKey(kibana kbtype.Kibana) string {
	return fmt.Sprintf("%s-%s-es-auth-secret", kibana.Namespace, kibana.Name)
}

func secretWatchFinalizer(kibana kbtype.Kibana, watches watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "es-auth-secret.finalizers.kibana.k8s.elastic.co",
		Execute: func() error {
			watches.Secrets.RemoveHandlerForKey(secretWatchKey(kibana))
			return nil
		},
	}
}

func (d *driver) deploymentParams(kb *kbtype.Kibana) (*DeploymentParams, error) {
	// setup a keystore with secure settings in an init container, if specified by the user
	volumes, initContainers, secureSettingsVersion, err := securesettings.Resources(
		d.client,
		d.recorder,
		d.dynamicWatches,
		"kibana-keystore",
		kb,
		k8s.ExtractNamespacedName(kb),
		kb.Spec.SecureSettings,
		volume.SecureSettingsVolumeName,
		volume.SecureSettingsVolumeMountPath,
		volume.KibanaDataVolume.VolumeMount(),
	)
	if err != nil {
		return nil, err
	}

	kibanaPodSpec := pod.NewPodTemplateSpec(*kb, volumes, initContainers)

	// Build a checksum of the configuration, which we can use to cause the Deployment to roll Kibana
	// instances in case of any change in the CA file, secure settings or credentials contents.
	// This is done because Kibana does not support updating those without restarting the process.
	configChecksum := sha256.New224()
	configChecksum.Write([]byte(secureSettingsVersion))

	// we need to deref the secret here (if any) to include it in the checksum otherwise Kibana will not be rolled on contents changes
	if kb.Spec.Elasticsearch.Auth.SecretKeyRef != nil {
		ref := kb.Spec.Elasticsearch.Auth.SecretKeyRef
		esAuthSecret := types.NamespacedName{Name: ref.Name, Namespace: kb.Namespace}
		if err := d.dynamicWatches.Secrets.AddHandler(watches.NamedWatch{
			Name:    secretWatchKey(*kb),
			Watched: esAuthSecret,
			Watcher: k8s.ExtractNamespacedName(kb),
		}); err != nil {
			return nil, err
		}
		sec := corev1.Secret{}
		if err := d.client.Get(esAuthSecret, &sec); err != nil {
			return nil, err
		}
		configChecksum.Write(sec.Data[ref.Key])
	} else {
		d.dynamicWatches.Secrets.RemoveHandlerForKey(secretWatchKey(*kb))
	}

	if kb.Spec.Elasticsearch.CertificateAuthorities.SecretName != "" {

		var esPublicCASecret corev1.Secret
		key := types.NamespacedName{Namespace: kb.Namespace, Name: kb.Spec.Elasticsearch.CertificateAuthorities.SecretName}
		// watch for changes in the CA secret
		if err := d.dynamicWatches.Secrets.AddHandler(watches.NamedWatch{
			Name:    secretWatchKey(*kb),
			Watched: key,
			Watcher: k8s.ExtractNamespacedName(kb),
		}); err != nil {
			return nil, err
		}

		if err := d.client.Get(key, &esPublicCASecret); err != nil {
			return nil, err
		}
		if certPem, ok := esPublicCASecret.Data[certificates.CertFileName]; ok {
			configChecksum.Write(certPem)
		}

		// TODO: this is a little ugly as it reaches into the ES controller bits
		esCertsVolume := es.CaCertSecretVolume(*kb)
		configVolume := config.SecretVolume(*kb)

		kibanaPodSpec.Spec.Volumes = append(kibanaPodSpec.Spec.Volumes,
			esCertsVolume.Volume(), configVolume.Volume())

		for i, container := range kibanaPodSpec.Spec.InitContainers {
			kibanaPodSpec.Spec.InitContainers[i].VolumeMounts = append(container.VolumeMounts,
				esCertsVolume.VolumeMount())
		}

		kibanaContainer := pod.GetKibanaContainer(kibanaPodSpec.Spec)
		kibanaContainer.VolumeMounts = append(kibanaContainer.VolumeMounts,
			esCertsVolume.VolumeMount(), configVolume.VolumeMount())
	}

	if kb.Spec.HTTP.TLS.Enabled() {
		// fetch the secret to calculate the checksum
		var httpCerts corev1.Secret
		err := d.client.Get(types.NamespacedName{
			Namespace: kb.Namespace,
			Name:      certificates.HTTPCertsInternalSecretName(kbname.KBNamer, kb.Name),
		}, &httpCerts)
		if err != nil {
			return nil, err
		}
		if httpCert, ok := httpCerts.Data[certificates.CertFileName]; ok {
			configChecksum.Write(httpCert)
		}

		// add volume/mount for http certs to pod spec
		httpCertsVolume := kbcerts.HTTPCertSecretVolume(*kb)
		kibanaPodSpec.Spec.Volumes = append(kibanaPodSpec.Spec.Volumes, httpCertsVolume.Volume())
		kibanaContainer := pod.GetKibanaContainer(kibanaPodSpec.Spec)
		kibanaContainer.VolumeMounts = append(kibanaContainer.VolumeMounts, httpCertsVolume.VolumeMount())

	}

	// get config secret to add its content to the config checksum
	configSecret := corev1.Secret{}
	err = d.client.Get(types.NamespacedName{Name: config.SecretName(*kb), Namespace: kb.Namespace}, &configSecret)
	if err != nil {
		return nil, err
	}
	configChecksum.Write(configSecret.Data[config.SettingsFilename])

	// add the checksum to a label for the deployment and its pods (the important bit is that the pod template
	// changes, which will trigger a rolling update)
	kibanaPodSpec.Labels[configChecksumLabel] = fmt.Sprintf("%x", configChecksum.Sum(nil))

	return &DeploymentParams{
		Name:            kbname.KBNamer.Suffix(kb.Name),
		Namespace:       kb.Namespace,
		Replicas:        kb.Spec.NodeCount,
		Selector:        label.NewLabels(kb.Name),
		Labels:          label.NewLabels(kb.Name),
		PodTemplateSpec: kibanaPodSpec,
	}, nil
}

func (d *driver) Reconcile(
	state *State,
	kb *kbtype.Kibana,
	params operator.Parameters,
) *reconciler.Results {
	results := reconciler.Results{}
	if !kb.Spec.Elasticsearch.IsConfigured() {
		log.Info("Aborting Kibana deployment reconciliation as no Elasticsearch backend is configured")
		return &results
	}

	svc, err := common.ReconcileService(d.client, d.scheme, NewService(*kb), kb)
	if err != nil {
		// TODO: consider updating some status here?
		return results.WithError(err)
	}

	results.WithResults(kbcerts.Reconcile(d.client, d.scheme, *kb, d.dynamicWatches, []corev1.Service{*svc}, params.CACertRotation))
	if results.HasError() {
		return &results
	}

	kbSettings, err := config.NewConfigSettings(d.client, *kb)
	if err != nil {
		return results.WithError(err)
	}
	err = kbSettings.MergeWith(
		settings.MustCanonicalConfig(d.settingsFactory(*kb)),
	)
	if err != nil {
		return results.WithError(err)
	}
	err = config.ReconcileConfigSecret(d.client, *kb, kbSettings, params.OperatorInfo)
	if err != nil {
		return results.WithError(err)
	}

	deploymentParams, err := d.deploymentParams(kb)
	if err != nil {
		return results.WithError(err)
	}
	expectedDp := NewDeployment(*deploymentParams)
	reconciledDp, err := ReconcileDeployment(d.client, d.scheme, expectedDp, kb)
	if err != nil {
		return results.WithError(err)
	}
	state.UpdateKibanaState(reconciledDp)
	return &results
}

func newDriver(
	client k8s.Client,
	scheme *runtime.Scheme,
	version version.Version,
	watches watches.DynamicWatches,
	recorder record.EventRecorder,
) (*driver, error) {
	d := driver{
		client:         client,
		scheme:         scheme,
		dynamicWatches: watches,
		recorder:       recorder,
	}
	switch version.Major {
	case 6:
		switch {
		case version.Minor >= 6:
			// 6.6 docker container already defaults to v7 settings
			d.settingsFactory = version7.SettingsFactory
		default:
			d.settingsFactory = version6.SettingsFactory
		}
	case 7:
		d.settingsFactory = version7.SettingsFactory
	default:
		return nil, fmt.Errorf("unsupported version: %s", version)
	}
	return &d, nil
}
