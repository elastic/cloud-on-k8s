// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	InitContainerConfigVolumeMountPath = "/mnt/elastic-internal/logstash-config-local"
	InitConfigContainerName            = "elastic-internal-init-config"

	// InitConfigScript is a small bash script to prepare the logstash configuration directory
	InitConfigScript = `#!/usr/bin/env bash
set -eux

init_config_initialized_flag=` + InitContainerConfigVolumeMountPath + `/elastic-internal-init-config.ok

if [[ -f "${init_config_initialized_flag}" ]]; then
    echo "Logstash configuration already initialized."
	exit 0
fi

echo "Setup Logstash configuration"

mount_path=` + InitContainerConfigVolumeMountPath + `

for f in /usr/share/logstash/config/*.*; do
	filename=$(basename $f)
	if [[ ! -f "$mount_path/$filename" ]]; then
		cp $f $mount_path
	fi
done

touch "${init_config_initialized_flag}"
echo "Logstash configuration successfully prepared."
`
)

// initConfigContainer returns an init container that executes a bash script to prepare the logstash config directory.
// The script copy config files from /use/share/logstash/config to /mnt/elastic-internal/logstash-config/
// TODO may be able to solve env2yaml permission issue with initContainer
func initConfigContainer() corev1.Container {
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
			{
				MountPath: InitContainerConfigVolumeMountPath,
				Name:      ConfigVolumeName,
			},
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
