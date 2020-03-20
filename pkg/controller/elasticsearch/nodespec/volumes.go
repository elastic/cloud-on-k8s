// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	esvolume "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

var downwardAPIVolume = volume.DownwardAPI{}

func buildVolumes(esName string, nodeSpec esv1.NodeSet, keystoreResources *keystore.Resources) ([]corev1.Volume, []corev1.VolumeMount) {

	configVolume := settings.ConfigSecretVolume(esv1.StatefulSet(esName, nodeSpec.Name))
	probeSecret := volume.NewSelectiveSecretVolumeWithMountPath(
		esv1.InternalUsersSecret(esName), esvolume.ProbeUserVolumeName,
		esvolume.ProbeUserSecretMountPath, []string{user.ProbeUserName},
	)
	httpCertificatesVolume := volume.NewSecretVolumeWithMountPath(
		certificates.HTTPCertsInternalSecretName(esv1.ESNamer, esName),
		esvolume.HTTPCertificatesSecretVolumeName,
		esvolume.HTTPCertificatesSecretVolumeMountPath,
	)
	transportCertificatesVolume := transportCertificatesVolume(esName)
	remoteCertificateAuthoritiesVolume := volume.NewSecretVolumeWithMountPath(
		esv1.RemoteCaSecretName(esName),
		esvolume.RemoteCertificateAuthoritiesSecretVolumeName,
		esvolume.RemoteCertificateAuthoritiesSecretVolumeMountPath,
	)
	unicastHostsVolume := volume.NewConfigMapVolume(
		esv1.UnicastHostsConfigMap(esName), esvolume.UnicastHostsVolumeName, esvolume.UnicastHostsVolumeMountPath,
	)
	usersSecretVolume := volume.NewSecretVolumeWithMountPath(
		esv1.RolesAndFileRealmSecret(esName),
		esvolume.XPackFileRealmVolumeName,
		esvolume.XPackFileRealmVolumeMountPath,
	)
	scriptsVolume := volume.NewConfigMapVolumeWithMode(
		esv1.ScriptsConfigMap(esName),
		esvolume.ScriptsVolumeName,
		esvolume.ScriptsVolumeMountPath,
		0755)

	// append future volumes from PVCs (not resolved to a claim yet)
	persistentVolumes := make([]corev1.Volume, 0, len(nodeSpec.VolumeClaimTemplates))
	for _, claimTemplate := range nodeSpec.VolumeClaimTemplates {
		persistentVolumes = append(persistentVolumes, corev1.Volume{
			Name: claimTemplate.Name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					// actual claim name will be resolved and fixed right before pod creation
					ClaimName: "claim-name-placeholder",
				},
			},
		})
	}

	volumes := append(
		persistentVolumes, // includes the data volume, unless specified differently in the pod template
		append(
			initcontainer.PluginVolumes.Volumes(),
			esvolume.DefaultLogsVolume,
			usersSecretVolume.Volume(),
			unicastHostsVolume.Volume(),
			probeSecret.Volume(),
			transportCertificatesVolume.Volume(),
			remoteCertificateAuthoritiesVolume.Volume(),
			httpCertificatesVolume.Volume(),
			scriptsVolume.Volume(),
			configVolume.Volume(),
			downwardAPIVolume.Volume(),
		)...)
	if keystoreResources != nil {
		volumes = append(volumes, keystoreResources.Volume)
	}

	volumeMounts := append(
		initcontainer.PluginVolumes.EsContainerVolumeMounts(),
		esvolume.DefaultDataVolumeMount,
		esvolume.DefaultLogsVolumeMount,
		usersSecretVolume.VolumeMount(),
		unicastHostsVolume.VolumeMount(),
		probeSecret.VolumeMount(),
		transportCertificatesVolume.VolumeMount(),
		remoteCertificateAuthoritiesVolume.VolumeMount(),
		httpCertificatesVolume.VolumeMount(),
		scriptsVolume.VolumeMount(),
		configVolume.VolumeMount(),
		downwardAPIVolume.VolumeMount(),
	)

	return volumes, volumeMounts
}
