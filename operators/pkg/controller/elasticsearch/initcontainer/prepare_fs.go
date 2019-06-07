// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
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
func NewPrepareFSInitContainer(
	imageName string,
	linkedFiles LinkedFilesArray,
	transportCertificatesVolume volume.SecretVolume,
) (corev1.Container, error) {
	privileged := false
	initContainerRunAsUser := defaultInitContainerRunAsUser

	// we mount the certificates to a location outside of the default config directory because the prepare-fs script
	// will attempt to move all the files under the configuration directory to a different volume, and it should not
	// be attempting to move files from this secret volume mount (any attempt to do so will be logged as errors).
	certificatesVolumeMount := transportCertificatesVolume.VolumeMount()
	certificatesVolumeMount.MountPath = "/volume/transport-certificates"

	script, err := RenderScriptTemplate(TemplateParams{
		Plugins:       defaultInstalledPlugins,
		SharedVolumes: PrepareFsSharedVolumes,
		LinkedFiles:   linkedFiles,
		ChownToElasticsearch: []string{
			DataSharedVolume.InitContainerMountPath,
			LogsSharedVolume.InitContainerMountPath,
		},
		TransportCertificatesKeyPath: fmt.Sprintf(
			"%s/%s", certificatesVolumeMount.MountPath, certificates.KeyFileName,
		),
	})
	if err != nil {
		return corev1.Container{}, err
	}

	container := corev1.Container{
		Image:           imageName,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            prepareFilesystemContainerName,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
			RunAsUser:  &initContainerRunAsUser,
		},
		Command: []string{"bash", "-c", script},
		VolumeMounts: append(
			PrepareFsSharedVolumes.InitContainerVolumeMounts(), certificatesVolumeMount,
		),
	}

	return container, nil
}
