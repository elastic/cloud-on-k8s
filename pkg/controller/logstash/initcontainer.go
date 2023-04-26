// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
)

const (
	InitConfigContainerName = "logstash-internal-init-config"

	// InitConfigScript is a small bash script to prepare the logstash configuration directory
	InitConfigScript = `#!/usr/bin/env bash
set -eu

init_config_initialized_flag=` + InitContainerConfigVolumeMountPath + `/elastic-internal-init-config.ok

if [[ -f "${init_config_initialized_flag}" ]]; then
    echo "Logstash configuration already initialized."
    exit 0
fi

echo "Setup Logstash configuration"

mount_path=` + InitContainerConfigVolumeMountPath + `

cp -f /usr/share/logstash/config/*.* "$mount_path"

ln -sf ` + InternalConfigVolumeMountPath + `/logstash.yml  $mount_path
ln -sf ` + InternalPipelineVolumeMountPath + `/pipelines.yml  $mount_path

touch "${init_config_initialized_flag}"
echo "Logstash configuration successfully prepared."
`
)

// initConfigContainer returns an init container that executes a bash script to prepare the logstash config directory.
// This copies files from the `config` folder of the docker image, and creates symlinks for the `logstash.yml` and
// `pipelines.yml` files created by the operator into a shared config folder to be used by the main logstash container.
// This enables dynamic reloads for `pipelines.yml`.
func initConfigContainer(ls logstashv1alpha1.Logstash) corev1.Container {
	privileged := false

	return corev1.Container{
		// Image will be inherited from pod template defaults
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            InitConfigContainerName,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
		Command: []string{"/usr/bin/env", "bash", "-c", InitConfigScript},
		VolumeMounts: []corev1.VolumeMount{
			ConfigSharedVolume.InitContainerVolumeMount(),
			ConfigVolume(ls).VolumeMount(),
			PipelineVolume(ls).VolumeMount(),
		},

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
