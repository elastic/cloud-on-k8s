// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user/filerealm"
	esvolume "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

const (
	initContainerTransportCertificatesVolumeMountPath = "/mnt/elastic-internal/transport-certificates"
)

// Volumes that are shared between the prepare-fs init container and the ES container
var (
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
		EsContainerMountPath:   esvolume.ConfigVolumeMountPath,
	}

	// EsPluginsSharedVolume contains the ES plugins/ directory
	EsPluginsSharedVolume = SharedVolume{
		Name:                   "elastic-internal-elasticsearch-plugins-local",
		InitContainerMountPath: "/mnt/elastic-internal/elasticsearch-plugins-local",
		EsContainerMountPath:   "/usr/share/elasticsearch/plugins",
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
				Source: stringsutil.Concat(esvolume.XPackFileRealmVolumeMountPath, "/", filerealm.UsersFile),
				Target: stringsutil.Concat(EsConfigSharedVolume.EsContainerMountPath, "/", filerealm.UsersFile),
			},
			{
				Source: stringsutil.Concat(esvolume.XPackFileRealmVolumeMountPath, "/", user.RolesFile),
				Target: stringsutil.Concat(EsConfigSharedVolume.EsContainerMountPath, "/", user.RolesFile),
			},
			{
				Source: stringsutil.Concat(esvolume.XPackFileRealmVolumeMountPath, "/", filerealm.UsersRolesFile),
				Target: stringsutil.Concat(EsConfigSharedVolume.EsContainerMountPath, "/", filerealm.UsersRolesFile),
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
	certificatesVolumeMount.MountPath = initContainerTransportCertificatesVolumeMountPath

	scriptsVolume := volume.NewConfigMapVolumeWithMode(
		esv1.ScriptsConfigMap(clusterName),
		esvolume.ScriptsVolumeName,
		esvolume.ScriptsVolumeMountPath,
		0755)

	privileged := false
	container := corev1.Container{
		Image:           imageName,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            PrepareFilesystemContainerName,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
		Env:     defaults.PodDownwardEnvVars(),
		Command: []string{"bash", "-c", path.Join(esvolume.ScriptsVolumeMountPath, PrepareFsScriptConfigKey)},
		VolumeMounts: append(
			PluginVolumes.InitContainerVolumeMounts(),
			certificatesVolumeMount,
			scriptsVolume.VolumeMount(),
			esvolume.DefaultDataVolumeMount,
			esvolume.DefaultLogsVolumeMount,
		),
		Resources: defaultResources,
	}

	return container, nil
}

func RenderPrepareFsScript() (string, error) {
	return RenderScriptTemplate(TemplateParams{
		PluginVolumes: PluginVolumes,
		LinkedFiles:   linkedFiles,
		ChownToElasticsearch: []string{
			esvolume.ElasticsearchDataMountPath,
			esvolume.ElasticsearchLogsMountPath,
		},
		InitContainerTransportCertificatesSecretVolumeMountPath: initContainerTransportCertificatesVolumeMountPath,
		InitContainerNodeTransportCertificatesKeyPath: path.Join(
			EsConfigSharedVolume.InitContainerMountPath,
			esvolume.NodeTransportCertificatePathSegment,
			esvolume.NodeTransportCertificateKeyFile,
		),
		InitContainerNodeTransportCertificatesCertPath: path.Join(
			EsConfigSharedVolume.InitContainerMountPath,
			esvolume.NodeTransportCertificatePathSegment,
			esvolume.NodeTransportCertificateCertFile,
		),
		TransportCertificatesSecretVolumeMountPath: esvolume.TransportCertificatesSecretVolumeMountPath,
	})
}
