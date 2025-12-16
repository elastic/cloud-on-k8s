// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	corev1 "k8s.io/api/core/v1"

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

// BuildVolumesParams contains parameters for building pod volumes.
type BuildVolumesParams struct {
	ESName string
	// Version is the Elasticsearch version.
	Version version.Version
	// NodeSpec is the node set specification.
	NodeSpec esv1.NodeSet
	// KeystoreResources is the keystore resources for the init container approach (pre-9.3).
	// This is nil when using the reloadable keystore approach.
	KeystoreResources *keystore.Resources
	// KeystoreSecretName is the name of the keystore Secret for the reloadable keystore approach (9.3+).
	// This is empty when using the init container approach.
	KeystoreSecretName string
	// DownwardAPIVolume is the downward API volume for node labels.
	DownwardAPIVolume volume.DownwardAPI
	// AdditionalMountsFromPolicy contains additional volume mounts from stack config policy.
	AdditionalMountsFromPolicy []volume.VolumeLike
}

func buildVolumes(params BuildVolumesParams) ([]corev1.Volume, []corev1.VolumeMount) {
	esName := params.ESName
	nodeSpec := params.NodeSpec
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
			params.DownwardAPIVolume.Volume(),
			tmpVolume.Volume(),
		)...)

	// Add keystore volume: either secure settings for init container (pre-9.3) or keystore secret (9.3+)
	if params.KeystoreResources != nil {
		// Init container approach: mount secure settings
		volumes = append(volumes, params.KeystoreResources.Volume)
	}
	if params.KeystoreSecretName != "" {
		// Reloadable keystore approach: mount the keystore secret
		keystoreSecretVolume := volume.NewSecretVolumeWithMountPath(
			params.KeystoreSecretName,
			esvolume.KeystoreSecretVolumeName,
			esvolume.KeystoreSecretVolumeMountPath,
		)
		volumes = append(volumes, keystoreSecretVolume.Volume())
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
		params.DownwardAPIVolume.VolumeMount(),
		tmpVolume.VolumeMount(),
	)

	// Add keystore secret mount for reloadable keystore (9.3+)
	// Note: we don't mount it to the ES container directly - the prepare-fs init container
	// creates a symlink from the secret mount to the config directory
	if params.KeystoreSecretName != "" {
		keystoreSecretVolume := volume.NewSecretVolumeWithMountPath(
			params.KeystoreSecretName,
			esvolume.KeystoreSecretVolumeName,
			esvolume.KeystoreSecretVolumeMountPath,
		)
		volumeMounts = append(volumeMounts, keystoreSecretVolume.VolumeMount())
	}

	// version gate for the file-based settings volume and volumeMounts
	if params.Version.GTE(filesettings.FileBasedSettingsMinPreVersion) {
		volumes = append(volumes, fileSettingsVolume.Volume())
		volumeMounts = append(volumeMounts, fileSettingsVolume.VolumeMount())
	}

	// additional volumes from stack config policy
	for _, vol := range params.AdditionalMountsFromPolicy {
		volumes = append(volumes, vol.Volume())
		volumeMounts = append(volumeMounts, vol.VolumeMount())
	}

	// include the user-provided PodTemplate volumes as the user may have defined the data volume there (e.g.: emptyDir or hostpath volume)
	volumeMounts = esvolume.AppendDefaultDataVolumeMount(volumeMounts, append(volumes, nodeSpec.PodTemplate.Spec.Volumes...))

	return volumes, volumeMounts
}
