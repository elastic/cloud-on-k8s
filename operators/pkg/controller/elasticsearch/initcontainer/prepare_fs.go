// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	corev1 "k8s.io/api/core/v1"
)

// Volumes that are shared between the prepare-fs init container and the ES container
var (
	DataSharedVolume = SharedVolume{
		Name:                   "data",
		InitContainerMountPath: "/volume/data",
		EsContainerMountPath:   "/usr/share/elasticsearch/data",
	}

	LogsSharedVolume = SharedVolume{
		Name:                   "logs",
		InitContainerMountPath: "/volume/logs",
		EsContainerMountPath:   "/usr/share/elasticsearch/logs",
	}

	EsBinSharedVolume = SharedVolume{
		Name:                   "bin-volume",
		InitContainerMountPath: "/volume/bin",
		EsContainerMountPath:   "/usr/share/elasticsearch/bin",
	}

	PrepareFsSharedVolumes = SharedVolumeArray{
		Array: []SharedVolume{
			// Contains configuration (elasticsearch.yml) and plugins configuration subdirs
			SharedVolume{
				Name:                   "config-volume",
				InitContainerMountPath: "/volume/config",
				EsContainerMountPath:   "/usr/share/elasticsearch/config",
			},
			// Contains plugins data
			SharedVolume{
				Name:                   "plugins-volume",
				InitContainerMountPath: "/volume/plugins",
				EsContainerMountPath:   "/usr/share/elasticsearch/plugins",
			},
			// Plugins may have binaries installed in /bin
			EsBinSharedVolume,
			DataSharedVolume,
			LogsSharedVolume,
		},
	}
)

// NewPrepareFSInitContainer creates an init container to handle things such as:
// - plugins installation
// - configuration changes
// Modified directories and files are meant to be persisted for reuse in the actual ES container.
// This container does not need to be privileged.
func NewPrepareFSInitContainer(imageName string, linkedFiles LinkedFilesArray) (corev1.Container, error) {
	privileged := false
	initContainerRunAsUser := defaultInitContainerRunAsUser
	script, err := RenderScriptTemplate(TemplateParams{
		Plugins:       defaultInstalledPlugins,
		SharedVolumes: PrepareFsSharedVolumes,
		LinkedFiles:   linkedFiles,
		ChownToElasticsearch: []string{
			DataSharedVolume.InitContainerMountPath,
			LogsSharedVolume.InitContainerMountPath,
		},
	})
	if err != nil {
		return corev1.Container{}, err
	}
	container := corev1.Container{
		Image:           imageName,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            "prepare-fs",
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
			RunAsUser:  &initContainerRunAsUser,
		},
		Command:      []string{"bash", "-c", script},
		VolumeMounts: PrepareFsSharedVolumes.InitContainerVolumeMounts(),
	}
	return container, nil
}
