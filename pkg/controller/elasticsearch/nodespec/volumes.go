// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	esvolume "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
)

func buildVolumes(
	esName string,
	version version.Version,
	nodeSpec esv1.NodeSet,
	keystoreResources *keystore.Resources,
	downwardAPIVolume volume.DownwardAPI,
) ([]corev1.Volume, []corev1.VolumeMount) {
	configVolume := settings.ConfigSecretVolume(esv1.StatefulSet(esName, nodeSpec.Name))
	probeSecret := volume.NewSelectiveSecretVolumeWithMountPath(
		esv1.InternalUsersSecret(esName), esvolume.ProbeUserVolumeName,
		esvolume.PodMountedUsersSecretMountPath, []string{user.ProbeUserName, user.PreStopUserName},
	)
	httpCertificatesVolume := volume.NewSecretVolumeWithMountPath(
		certificates.InternalCertsSecretName(esv1.ESNamer, esName),
		esvolume.HTTPCertificatesSecretVolumeName,
		esvolume.HTTPCertificatesSecretVolumeMountPath,
	)
	transportCertificatesVolume := transportCertificatesVolume(esv1.StatefulSet(esName, nodeSpec.Name))
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
	fileSettingsVolume := volume.NewSecretVolumeWithMountPath(
		esv1.FileSettingsSecretName(esName),
		esvolume.FileSettingsVolumeName,
		esvolume.FileSettingsVolumeMountPath,
	)
	tmpVolume := volume.NewEmptyDirVolume(
		esvolume.TempVolumeName,
		esvolume.TempVolumeMountPath,
	)
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

	volumes := persistentVolumes
	volumes = append(
		volumes, // includes the data volume, unless specified differently in the pod template
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
			tmpVolume.Volume(),
		)...)
	if keystoreResources != nil {
		volumes = append(volumes, keystoreResources.Volume)
	}

	volumeMounts := append(
		initcontainer.PluginVolumes.ContainerVolumeMounts(),
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
		tmpVolume.VolumeMount(),
	)

	// version gate for the file-based settings volume and volumeMounts
	if version.GTE(filesettings.FileBasedSettingsMinPreVersion) {
		volumes = append(volumes, fileSettingsVolume.Volume())
		volumeMounts = append(volumeMounts, fileSettingsVolume.VolumeMount())
	}

	// include the user-provided PodTemplate volumes as the user may have defined the data volume there (e.g.: emptyDir or hostpath volume)
	volumeMounts = esvolume.AppendDefaultDataVolumeMount(volumeMounts, append(volumes, nodeSpec.PodTemplate.Spec.Volumes...))

	return volumes, volumeMounts
}
