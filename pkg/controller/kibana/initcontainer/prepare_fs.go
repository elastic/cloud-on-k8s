// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
)

const (
	ScriptsVolumeMountPath = "/mnt/elastic-internal/scripts"
)

var (
	// PluginsSharedVolume contains the Kibana plugins/ directory
	PluginsSharedVolume = volume.SharedVolume{
		// This volume name is the same as the primary container's volume name
		// so that the init container does not mount the plugins emptydir volume
		// on top of /usr/share/kibana/plugins.
		VolumeName:             "kibana-plugins",
		InitContainerMountPath: "/mnt/elastic-internal/kibana-plugins-local",
		ContainerMountPath:     "/usr/share/kibana/plugins",
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

// NewPreparePluginsInitContainer creates an init container to handle kibana plugins persistence.
func NewPreparePluginsInitContainer() (corev1.Container, error) {
	container := corev1.Container{
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            PrepareFilesystemContainerName,
		Env:             defaults.PodDownwardEnvVars(),
		Command:         []string{"bash", "-c", path.Join(ScriptsVolumeMountPath, PrepareFsScriptConfigKey)},
		VolumeMounts:    PluginVolumes.InitContainerVolumeMounts(),
		Resources:       defaultResources,
	}

	return container, nil
}

func RenderPrepareFsScript() (string, error) {
	templateParams := TemplateParams{
		ContainerPluginsMountPath:     PluginsSharedVolume.ContainerMountPath,
		InitContainerPluginsMountPath: PluginsSharedVolume.InitContainerMountPath,
	}
	return RenderScriptTemplate(templateParams)
}
