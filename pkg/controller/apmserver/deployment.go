// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"

	"go.elastic.co/apm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/pod"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func (r *ReconcileApmServer) reconcileApmServerDeployment(
	ctx context.Context,
	state State,
	as *apmv1.ApmServer,
) (State, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_deployment", tracing.SpanTypeApp)
	defer span.End()

	tokenSecret, err := reconcileApmServerToken(r.Client, as)
	if err != nil {
		return state, err
	}
	reconciledConfigSecret, err := reconcileApmServerConfig(r.Client, as)
	if err != nil {
		return state, err
	}

	keystoreResources, err := keystore.NewResources(
		r,
		as,
		Namer,
		NewLabels(as.Name),
		initContainerParameters,
	)
	if err != nil {
		return state, err
	}

	apmServerPodSpecParams := PodSpecParams{
		Version:         as.Spec.Version,
		CustomImageName: as.Spec.Image,

		PodTemplate: as.Spec.PodTemplate,

		TokenSecret:  tokenSecret,
		ConfigSecret: reconciledConfigSecret,

		keystoreResources: keystoreResources,
	}
	params, err := r.deploymentParams(as, apmServerPodSpecParams)
	if err != nil {
		return state, err
	}

	deploy := deployment.New(params)
	result, err := deployment.Reconcile(r.K8sClient(), deploy, as)
	if err != nil {
		return state, err
	}

	pods, err := k8s.PodsMatchingLabels(r.K8sClient(), as.Namespace, map[string]string{ApmServerNameLabelName: as.Name})
	if err != nil {
		return state, err
	}
	state.UpdateApmServerState(result, pods, tokenSecret)
	return state, nil
}

func (r *ReconcileApmServer) deploymentParams(
	as *apmv1.ApmServer,
	params PodSpecParams,
) (deployment.Params, error) {

	podSpec := newPodSpec(as, params)

	// Build a checksum of the configuration, the keystore, and the cert files used by ES and Kibana.
	// The checksum is added to the pod labels so a change triggers a rolling update. This is done because Apm Server
	// does not support updating its configuration file or the CA file contents without restarting the process.
	configChecksum := sha256.New224()
	_, _ = configChecksum.Write(params.ConfigSecret.Data[ApmCfgSecretKey])
	if params.keystoreResources != nil {
		_, _ = configChecksum.Write([]byte(params.keystoreResources.Version))
	}

	for _, association := range as.GetAssociations() {
		if association.AssociationConf().CAIsConfigured() {
			caSecretName := association.AssociationConf().GetCASecretName()

			var publicCASecret corev1.Secret
			key := types.NamespacedName{Namespace: as.Namespace, Name: caSecretName}
			if err := r.Get(context.Background(), key, &publicCASecret); err != nil {
				return deployment.Params{}, err
			}
			if certPem, ok := publicCASecret.Data[certificates.CertFileName]; ok {
				_, _ = configChecksum.Write(certPem)
			}

			caVolume := volume.NewSecretVolumeWithMountPath(
				caSecretName,
				fmt.Sprintf("%s-certs", association.AssociationType()),
				filepath.Join(ApmBaseDir, certificatesDir(association.AssociationType())),
			)
			podSpec.Spec.Volumes = append(podSpec.Spec.Volumes, caVolume.Volume())

			for i := range podSpec.Spec.InitContainers {
				podSpec.Spec.InitContainers[i].VolumeMounts = append(podSpec.Spec.InitContainers[i].VolumeMounts, caVolume.VolumeMount())
			}

			for i := range podSpec.Spec.Containers {
				podSpec.Spec.Containers[i].VolumeMounts = append(podSpec.Spec.Containers[i].VolumeMounts, caVolume.VolumeMount())
			}
		}
	}

	if as.Spec.HTTP.TLS.Enabled() {
		// fetch the secret to calculate the checksum
		var httpCerts corev1.Secret
		err := r.Get(context.Background(), types.NamespacedName{
			Namespace: as.Namespace,
			Name:      certificates.InternalCertsSecretName(Namer, as.Name),
		}, &httpCerts)
		if err != nil {
			return deployment.Params{}, err
		}
		if httpCert, ok := httpCerts.Data[certificates.CertFileName]; ok {
			_, _ = configChecksum.Write(httpCert)
		}
		httpCertsVolume := certificates.HTTPCertSecretVolume(Namer, as.Name)
		podSpec.Spec.Volumes = append(podSpec.Spec.Volumes, httpCertsVolume.Volume())
		apmServerContainer := pod.ContainerByName(podSpec.Spec, apmv1.ApmServerContainerName)
		apmServerContainer.VolumeMounts = append(apmServerContainer.VolumeMounts, httpCertsVolume.VolumeMount())
	}

	// add secret token to hash to force pod rotation on change
	_, _ = configChecksum.Write(params.TokenSecret.Data[SecretTokenKey])

	podSpec.Labels[configChecksumLabelName] = fmt.Sprintf("%x", configChecksum.Sum(nil))

	return deployment.Params{
		Name:            Deployment(as.Name),
		Namespace:       as.Namespace,
		Replicas:        as.Spec.Count,
		Selector:        NewLabels(as.Name),
		Labels:          NewLabels(as.Name),
		PodTemplateSpec: podSpec,
		Strategy:        appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
	}, nil
}
