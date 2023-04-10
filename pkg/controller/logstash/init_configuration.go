// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	//"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
)

//const (
//	//InitContainerConfigVolumeMountPath = "/mnt/elastic-internal/logstash-config-local"
//	InitConfigContainerName            = "logstash-internal-init-config"
//
//	// InitConfigScript is a small bash script to prepare the logstash configuration directory
//	InitConfigScript = `#!/usr/bin/env bash
//set -eux
//
//init_config_initialized_flag=` + InitContainerConfigVolumeMountPath + `/elastic-internal-init-config.ok
//
//if [[ -f "${init_config_initialized_flag}" ]]; then
//    echo "Logstash configuration already initialized."
//	exit 0
//fi
//
//echo "Setup Logstash configuration"
//
//ln -sf /mnt/elastic-internal/logstash-pipeline/pipelines.yml /usr/share/logstash/config/
//ln -sf /mnt/elastic-internal/logstash-config/logstash.yml /usr/share/logstash/config/
//
//cp /usr/share/logstash/config/jvm.options /mnt/elastic-internal/logstash-config-local/jvm.options
//cp /usr/share/logstash/config/log4j2.properties /mnt/elastic-internal/logstash-config-local/log4j2.properties
//
//
//
//touch "${init_config_initialized_flag}"
//echo "Logstash configuration successfully prepared."
//`
//)

const (
	//InitContainerConfigVolumeMountPath = "/mnt/elastic-internal/logstash-config-local"
	InitConfigContainerName            = "logstash-internal-init-config"

	// InitConfigScript is a small bash script to prepare the logstash configuration directory
	InitConfigScript = `#!/usr/bin/env bash
set -eux

init_config_initialized_flag=` + InitContainerConfigVolumeMountPath + `/elastic-internal-init-config.ok

if [[ -f "${init_config_initialized_flag}" ]]; then
    echo "Logstash configuration already initialized."
	exit 0
fi

echo "Setup Logstash configuration"

ls /mnt/elastic-internal/logstash-config/*

ln -sf /mnt/elastic-internal/logstash-config/*  /usr/share/logstash/config/
ln -sf /mnt/elastic-internal/logstash-pipeline/*  /usr/share/logstash/config/
ln -sf /mnt/elastic-internal/logstash-config/*  /mnt/elastic-internal/logstash-config-local/.
ln -sf /mnt/elastic-internal/logstash-pipeline/*  /mnt/elastic-internal/logstash-config-local/.


cp /usr/share/logstash/config/jvm.options /mnt/elastic-internal/logstash-config-local/.
cp /usr/share/logstash/config/log4j2.properties /mnt/elastic-internal/logstash-config-local/.


touch "${init_config_initialized_flag}"
echo "Logstash configuration successfully prepared."
`
)


//var (
//	LsConfigSharedVolume := volume.SharedVolume{
//		VolumeName:             ConfigVolumeName,
//		InitContainerMountPath: InitContainerConfigVolumeMountPath,
//		ContainerMountPath:     ConfigMountPath,
//	}
//
//	PluginVolumes = volume.SharedVolumeArray{
//		Array: []volume.SharedVolume{
//			LsConfigSharedVolume,
//			LsPipelineShareVolume,
//			EsBinSharedVolume,
//		},
//	}
//
//	ConfigVolumes := volume.SharedVolumeArray{
//	Array: []volume.SharedVolume{
//		VolumeMounts: []corev1.VolumeMount{
//			//ConfigSharedVolume.InitContainerVolumeMount(),
//			ConfigVolume(ls).VolumeMount(),
//			PipelineVolume(ls).VolumeMount(),
//		},
//
//		EsConfigSharedVolume,
//		EsPluginsSharedVolume,
//		EsBinSharedVolume,
//	},
//}
//)

// initConfigContainer returns an init container that executes a bash script to prepare the logstash config directory.
// The script copy config files from /use/share/logstash/config to /mnt/elastic-internal/logstash-config/
// TODO may be able to solve env2yaml permission issue with initContainer
func initConfigContainer(ls logstashv1alpha1.Logstash) corev1.Container {
	privileged := false
	//configVolumes := volume.SharedVolumeArray{
	//	Array: []volume.SharedVolume{
	//		EsConfigSharedVolume,
	//		EsPluginsSharedVolume,
	//		EsBinSharedVolume,
	//	},
	//}
	//volumeMounts :=
	//	// we will also inherit all volume mounts from the main container later on in the pod template builder
	//	configVolumes.InitContainerVolumeMounts()

	return corev1.Container{
		// Image will be inherited from pod template defaults
		Image: "docker.elastic.co/logstash/logstash:8.6.1",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            InitConfigContainerName,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
		Command: []string{"/usr/bin/env", "bash", "-c", InitConfigScript},
		//VolumeMounts: volumeMounts,
		VolumeMounts: []corev1.VolumeMount{
			ConfigSharedVolume.InitContainerVolumeMount(),
			ConfigVolume(ls).VolumeMount(),
			PipelineVolume(ls).VolumeMount(),
		},
		//PluginVolumes.InitContainerVolumeMounts(),

		Resources: corev1.ResourceRequirements{
			Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("50Mi"),
				corev1.ResourceCPU:    resource.MustParse("0.1"),
			},
			Limits: map[corev1.ResourceName]resource.Quantity{
				// Memory limit should be at least 12582912 when running with CRI-O
				corev1.ResourceMemory: resource.MustParse("50Mi"),
				corev1.ResourceCPU:    resource.MustParse("0.1"),
			},
		},
	}
}