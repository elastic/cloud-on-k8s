// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"fmt"
	"path"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	volume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/user"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
	corev1 "k8s.io/api/core/v1"
)

const (
	transportCertificatesVolumeMountPath = "/mnt/elastic-internal/transport-certificates"
)

// Volumes that are shared between the prepare-fs init container and the ES container
var (
	DataSharedVolume = SharedVolume{
		Name:                   esvolume.ElasticsearchDataVolumeName,
		InitContainerMountPath: settings.EsContainerDataMountPath,
		EsContainerMountPath:   settings.EsContainerDataMountPath,
	}

	LogsSharedVolume = SharedVolume{
		Name:                   esvolume.ElasticsearchLogsVolumeName,
		InitContainerMountPath: settings.EsContainerLogsMountPath,
		EsContainerMountPath:   settings.EsContainerLogsMountPath,
	}

	// EsBinSharedVolume contains the ES bin/ directory
	EsBinSharedVolume = SharedVolume{
		Name:                   "elastic-internal-elasticsearch-bin-local",
		InitContainerMountPath: "/mnt/elastic-internal/elasticsearch-bin-local",
		EsContainerMountPath:   "/usr/share/elasticsearch/bin",
	}

	// EsConfigSharedVolume contains the ES config/ directory
	EsConfigSharedVolume = SharedVolume{
		Name:                   "elastic-internal-elasticsearch-config-local",
		InitContainerMountPath: "/mnt/elastic-internal/elasticsearch-config-local",
		EsContainerMountPath:   "/usr/share/elasticsearch/config",
	}

	// EsPluginsSharedVolume contains the ES plugins/ directory
	EsPluginsSharedVolume = SharedVolume{
		Name:                   "elastic-internal-elasticsearch-plugins-local",
		InitContainerMountPath: "/mnt/elastic-internal/elasticsearch-plugins-local",
		EsContainerMountPath:   "/usr/share/elasticsearch/plugins",
	}

	PrepareFsSharedVolumes = SharedVolumeArray{
		Array: []SharedVolume{
			EsConfigSharedVolume,
			EsPluginsSharedVolume,
			EsBinSharedVolume,
			DataSharedVolume,
			LogsSharedVolume,
		},
	}

	PluginVolumes = SharedVolumeArray{
		Array: []SharedVolume{
			EsConfigSharedVolume,
			EsPluginsSharedVolume,
			EsBinSharedVolume,
		},
	}

	// linkedFiles describe how various secrets are mapped into the pod's filesystem.
	linkedFiles = LinkedFilesArray{
		Array: []LinkedFile{
			{
				Source: stringsutil.Concat(esvolume.XPackFileRealmVolumeMountPath, "/", user.ElasticUsersFile),
				Target: stringsutil.Concat(EsConfigSharedVolume.EsContainerMountPath, "/", user.ElasticUsersFile),
			},
			{
				Source: stringsutil.Concat(esvolume.XPackFileRealmVolumeMountPath, "/", user.ElasticRolesFile),
				Target: stringsutil.Concat(EsConfigSharedVolume.EsContainerMountPath, "/", user.ElasticRolesFile),
			},
			{
				Source: stringsutil.Concat(esvolume.XPackFileRealmVolumeMountPath, "/", user.ElasticUsersRolesFile),
				Target: stringsutil.Concat(EsConfigSharedVolume.EsContainerMountPath, "/", user.ElasticUsersRolesFile),
			},
			{
				Source: stringsutil.Concat(settings.ConfigVolumeMountPath, "/", settings.ConfigFileName),
				Target: stringsutil.Concat(EsConfigSharedVolume.EsContainerMountPath, "/", settings.ConfigFileName),
			},
			{
				Source: stringsutil.Concat(esvolume.UnicastHostsVolumeMountPath, "/", esvolume.UnicastHostsFile),
				Target: stringsutil.Concat(EsConfigSharedVolume.EsContainerMountPath, "/", esvolume.UnicastHostsFile),
			},
		},
	}
)

// NewPrepareFSInitContainer creates an init container to handle things such as:
// - configuration changes
// Modified directories and files are meant to be persisted for reuse in the actual ES container.
// This container does not need to be privileged.
func NewPrepareFSInitContainer(
	imageName string,
	transportCertificatesVolume volume.SecretVolume,
	clusterName string,
) (corev1.Container, error) {

	// we mount the certificates to a location outside of the default config directory because the prepare-fs script
	// will attempt to move all the files under the configuration directory to a different volume, and it should not
	// be attempting to move files from this secret volume mount (any attempt to do so will be logged as errors).
	certificatesVolumeMount := transportCertificatesVolume.VolumeMount()
	certificatesVolumeMount.MountPath = transportCertificatesVolumeMountPath

	scriptsVolume := volume.NewConfigMapVolumeWithMode(
		name.ScriptsConfigMap(clusterName),
		esvolume.ScriptsVolumeName,
		esvolume.ScriptsVolumeMountPath,
		0755)

	privileged := false
	container := corev1.Container{
		Image:           imageName,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            prepareFilesystemContainerName,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
		Command: []string{"bash", "-c", path.Join(esvolume.ScriptsVolumeMountPath, PrepareFsScriptConfigKey)},
		VolumeMounts: append(
			PrepareFsSharedVolumes.InitContainerVolumeMounts(), certificatesVolumeMount, scriptsVolume.VolumeMount(),
		),
	}

	return container, nil
}

func RenderPrepareFsScript() (string, error) {
	return RenderScriptTemplate(TemplateParams{
		PluginVolumes: PluginVolumes,
		LinkedFiles:   linkedFiles,
		ChownToElasticsearch: []string{
			DataSharedVolume.InitContainerMountPath,
			LogsSharedVolume.InitContainerMountPath,
		},
		TransportCertificatesKeyPath: fmt.Sprintf(
			"%s/%s",
			transportCertificatesVolumeMountPath,
			certificates.KeyFileName),
	})
}
