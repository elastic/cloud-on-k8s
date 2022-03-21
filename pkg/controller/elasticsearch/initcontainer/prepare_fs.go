// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

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
	EsBinSharedVolume = volume.SharedVolume{
		VolumeName:             "elastic-internal-elasticsearch-bin-local",
		InitContainerMountPath: "/mnt/elastic-internal/elasticsearch-bin-local",
		ContainerMountPath:     "/usr/share/elasticsearch/bin",
	}

	// EsConfigSharedVolume contains the ES config/ directory
	EsConfigSharedVolume = volume.SharedVolume{
		VolumeName:             "elastic-internal-elasticsearch-config-local",
		InitContainerMountPath: "/mnt/elastic-internal/elasticsearch-config-local",
		ContainerMountPath:     esvolume.ConfigVolumeMountPath,
	}

	// EsPluginsSharedVolume contains the ES plugins/ directory
	EsPluginsSharedVolume = volume.SharedVolume{
		VolumeName:             "elastic-internal-elasticsearch-plugins-local",
		InitContainerMountPath: "/mnt/elastic-internal/elasticsearch-plugins-local",
		ContainerMountPath:     "/usr/share/elasticsearch/plugins",
	}

	PluginVolumes = volume.SharedVolumeArray{
		Array: []volume.SharedVolume{
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
				Target: stringsutil.Concat(EsConfigSharedVolume.ContainerMountPath, "/", filerealm.UsersFile),
			},
			{
				Source: stringsutil.Concat(esvolume.XPackFileRealmVolumeMountPath, "/", user.RolesFile),
				Target: stringsutil.Concat(EsConfigSharedVolume.ContainerMountPath, "/", user.RolesFile),
			},
			{
				Source: stringsutil.Concat(esvolume.XPackFileRealmVolumeMountPath, "/", filerealm.UsersRolesFile),
				Target: stringsutil.Concat(EsConfigSharedVolume.ContainerMountPath, "/", filerealm.UsersRolesFile),
			},
			{
				Source: stringsutil.Concat(settings.ConfigVolumeMountPath, "/", settings.ConfigFileName),
				Target: stringsutil.Concat(EsConfigSharedVolume.ContainerMountPath, "/", settings.ConfigFileName),
			},
			{
				Source: stringsutil.Concat(esvolume.UnicastHostsVolumeMountPath, "/", esvolume.UnicastHostsFile),
				Target: stringsutil.Concat(EsConfigSharedVolume.ContainerMountPath, "/", esvolume.UnicastHostsFile),
			},
			{
				Source: stringsutil.Concat(esvolume.XPackFileRealmVolumeMountPath, "/", esvolume.ServiceAccountsFile),
				Target: stringsutil.Concat(EsConfigSharedVolume.ContainerMountPath, "/", esvolume.ServiceAccountsFile),
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
func NewPrepareFSInitContainer(transportCertificatesVolume volume.SecretVolume, nodeLabelsAsAnnotations []string) (corev1.Container, error) {
	// we mount the certificates to a location outside of the default config directory because the prepare-fs script
	// will attempt to move all the files under the configuration directory to a different volume, and it should not
	// be attempting to move files from this secret volume mount (any attempt to do so will be logged as errors).
	certificatesVolumeMount := transportCertificatesVolume.VolumeMount()
	certificatesVolumeMount.MountPath = initContainerTransportCertificatesVolumeMountPath

	privileged := false
	volumeMounts := append(
		// we will also inherit all volume mounts from the main container later on in the pod template builder
		PluginVolumes.InitContainerVolumeMounts(),
		certificatesVolumeMount,
	)
	if len(nodeLabelsAsAnnotations) > 0 {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      esvolume.DownwardAPIVolumeName,
			ReadOnly:  true,
			MountPath: esvolume.DownwardAPIMountPath,
		})
	}
	container := corev1.Container{
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            PrepareFilesystemContainerName,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
		Env:          defaults.PodDownwardEnvVars(),
		Command:      []string{"bash", "-c", path.Join(esvolume.ScriptsVolumeMountPath, PrepareFsScriptConfigKey)},
		VolumeMounts: volumeMounts,
		Resources:    defaultResources,
	}

	return container, nil
}

func RenderPrepareFsScript(expectedAnnotations []string) (string, error) {
	templateParams := TemplateParams{
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
	}
	if len(expectedAnnotations) > 0 {
		expectedAnnotationsAsString := strings.Join(expectedAnnotations, " ")
		templateParams.ExpectedAnnotations = &expectedAnnotationsAsString
	}
	return RenderScriptTemplate(templateParams)
}
