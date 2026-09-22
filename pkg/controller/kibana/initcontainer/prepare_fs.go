// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	kbvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/volume"
)

var (
	// ConfigSharedVolume contains the Kibana config/ directory, it's an empty volume where the required configuration
	// is initialized by the elastic-internal-init init container. Its content is then shared by the init container
	// that creates the keystore and the main Kibana container.
	// This is needed in order to have in a same directory both the generated configuration and the keystore file which
	// is created in /usr/share/kibana/config since Kibana 7.9
	ConfigSharedVolume = volume.SharedVolume{
		VolumeName:             kbvolume.ConfigVolumeName,
		InitContainerMountPath: kbvolume.InitContainerConfigVolumeMountPath,
		ContainerMountPath:     kbvolume.ConfigVolumeMountPath,
	}

	// PluginsSharedVolume contains the Kibana plugins/ directory
	PluginsSharedVolume = volume.SharedVolume{
		// This volume name is the same as the primary container's volume name
		// so that the init container does not mount the plugins emptydir volume
		// on top of /usr/share/kibana/plugins.
		VolumeName:             kbvolume.PluginsVolumeName,
		InitContainerMountPath: kbvolume.PluginsVolumeInternalMountPath,
		ContainerMountPath:     kbvolume.PluginsVolumeMountPath,
	}

	PluginVolumes = volume.SharedVolumeArray{
		Array: []volume.SharedVolume{
			PluginsSharedVolume,
		},
	}

	// defaultResources are the default request and limits for the init container.
	defaultResources = corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("50Mi"),
			corev1.ResourceCPU:    resource.MustParse("0.1"),
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			// Memory limit should be at least 12582912 when running with CRI-O
			corev1.ResourceMemory: resource.MustParse("50Mi"),
			corev1.ResourceCPU:    resource.MustParse("0.1"),
		},
	}
)

// ConfigVolume returns a SecretVolume to hold the Kibana config identified by configSecretName.
func ConfigVolume(configSecretName string) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		configSecretName,
		kbvolume.InternalConfigVolumeName,
		kbvolume.InternalConfigVolumeMountPath,
	)
}

// NewInitContainer creates an init container to handle kibana configuration and plugins persistence.
// configSecretName is the name of the Secret that holds kibana.yml for this pool.
func NewInitContainer(configSecretName string) (corev1.Container, error) {
	container := corev1.Container{
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            kbvolume.InitContainerName,
		Env:             defaults.PodDownwardEnvVars(),
		Command:         []string{"/usr/bin/env", "bash", "-c", path.Join(kbvolume.ScriptsVolumeMountPath, KibanaInitScriptConfigKey)},
		VolumeMounts: []corev1.VolumeMount{
			ConfigSharedVolume.InitContainerVolumeMount(),
			ConfigVolume(configSecretName).VolumeMount(),
			PluginsSharedVolume.InitContainerVolumeMount(),
		},
		Resources: defaultResources,
	}

	return container, nil
}

// renderInitScript renders the init script that will be run by the init container.
func renderInitScript() (string, error) {
	templateParams := templateParams{
		ContainerPluginsMountPath:     PluginsSharedVolume.ContainerMountPath,
		InitContainerPluginsMountPath: PluginsSharedVolume.InitContainerMountPath,
	}
	return renderScriptTemplate(templateParams)
}
