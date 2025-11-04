// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/stringsutil"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	esvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
)

func buildVolumes(
	esName string,
	isStateless bool,
	version version.Version,
	nodeSetSpec esv1.NodeSetSpec,
	keystoreResources *keystore.Resources,
	downwardAPIVolume volume.DownwardAPI,
	additionalMountsFromPolicy []volume.VolumeLike,
) ([]corev1.Volume, []corev1.VolumeMount) {
	podControllerResourceName := esv1.PodsControllerResourceName(esName, nodeSetSpec.GetName())
	configVolume := settings.ConfigSecretVolume(podControllerResourceName)

	probeSecret := volume.NewSelectiveSecretVolumeWithMountPath(
		esv1.InternalUsersSecret(esName), esvolume.ProbeUserVolumeName,
		esvolume.PodMountedUsersSecretMountPath, []string{user.ProbeUserName, user.PreStopUserName},
	)
	httpCertificatesVolume := volume.NewSecretVolumeWithMountPath(
		certificates.InternalCertsSecretName(esv1.ESNamer, esName),
		esvolume.HTTPCertificatesSecretVolumeName,
		esvolume.HTTPCertificatesSecretVolumeMountPath,
	)
	transportCertificatesVolume := transportCertificatesVolume(podControllerResourceName)
	remoteCertificateAuthoritiesVolume := volume.NewSecretVolumeWithMountPath(
		esv1.RemoteCaSecretName(esName),
		esvolume.RemoteCertificateAuthoritiesSecretVolumeName,
		esvolume.RemoteCertificateAuthoritiesSecretVolumeMountPath,
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
	volumeTemplate := nodeSetSpec.GetVolumeClaimTemplates()
	persistentVolumes := make([]corev1.Volume, 0, len(volumeTemplate))
	for _, claimTemplate := range volumeTemplate {
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

	volumes := make([]corev1.Volume, 0, len(persistentVolumes)+len(volumeTemplate))
	if !isStateless {
		// CloneSet automatically adds PVC volumes in Spec.VolumeClaimTemplates, no need to add them again.
		// We still need to add the mount point later though...
		volumes = append(volumes, persistentVolumes...)
	}
	volumes = append(
		volumes, // includes the data volume, unless specified differently in the pod template
		append(
			initcontainer.PluginVolumes.Volumes(),
			esvolume.DefaultLogsVolume,
			usersSecretVolume.Volume(),
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
		probeSecret.VolumeMount(),
		transportCertificatesVolume.VolumeMount(),
		remoteCertificateAuthoritiesVolume.VolumeMount(),
		httpCertificatesVolume.VolumeMount(),
		scriptsVolume.VolumeMount(),
		configVolume.VolumeMount(),
		downwardAPIVolume.VolumeMount(),
		tmpVolume.VolumeMount(),
	)

	// Unicast is still required for cluster bootstrapping even for stateless nodes.
	unicastHostsVolume := volume.NewConfigMapVolume(
		esv1.UnicastHostsConfigMap(esName), esvolume.UnicastHostsVolumeName, esvolume.UnicastHostsVolumeMountPath,
	)
	volumes = append(volumes, unicastHostsVolume.Volume())
	volumeMounts = append(volumeMounts, unicastHostsVolume.VolumeMount())

	if isStateless {
		// mount again the config secret for each settings file watched by ES, with only the file projected in the volume.
		// If we handle these files like the other config files with symlinks created via the initcontainer, updates to these
		// files are not seen by ES.
		secureSettingsVolume := volume.NewSelectiveSecretVolumeWithMountPath(
			settings.ConfigSecretName(podControllerResourceName),
			settings.SecureSettingVolumeName,
			stringsutil.Concat(esvolume.ConfigVolumeMountPath, "/", settings.SecureSettingsDirName),
			[]string{settings.SecureSettingsFileName},
		)
		volumes = append(volumes, secureSettingsVolume.Volume())
		volumeMounts = append(volumeMounts, secureSettingsVolume.VolumeMount())
	}

	// version gate for the file-based settings volume and volumeMounts
	if isStateless || version.GTE(filesettings.FileBasedSettingsMinPreVersion) {
		volumes = append(volumes, fileSettingsVolume.Volume())
		volumeMounts = append(volumeMounts, fileSettingsVolume.VolumeMount())
	}

	// additional volumes from stack config policy
	for _, volume := range additionalMountsFromPolicy {
		volumes = append(volumes, volume.Volume())
		volumeMounts = append(volumeMounts, volume.VolumeMount())
	}

	podTemplate := nodeSetSpec.GetPodTemplate()
	// include the user-provided PodTemplate volumes as the user may have defined the data volume there (e.g.: emptyDir or hostpath volume)
	volumeMounts = esvolume.AppendDefaultDataVolumeMount(volumeMounts, append(volumes, podTemplate.Spec.Volumes...), persistentVolumes)

	return volumes, volumeMounts
}
