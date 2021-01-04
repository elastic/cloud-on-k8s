// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	InitConfigContainerName = "elastic-internal-init-config"

	// InitConfigScript is a small bash script to prepare the Kibana configuration directory
	InitConfigScript = `#!/usr/bin/env bash
set -eux

init_config_initialized_flag=` + InitContainerConfigVolumeMountPath + `/elastic-internal-init-config.ok

if [[ -f "${init_config_initialized_flag}" ]]; then
    echo "Kibana configuration already initialized."
	exit 0
fi

echo "Setup Kibana configuration"

ln -sf ` + InternalConfigVolumeMountPath + `/* ` + InitContainerConfigVolumeMountPath + `/

touch "${init_config_initialized_flag}"
echo "Kibana configuration successfully prepared."
`
)

// initConfigContainer returns an init container that executes a bash script to prepare the Kibana config directory.
// The script creates symbolic links from the generated configuration files in /mnt/elastic-internal/kibana-config/ to
// an empty directory later mounted in /use/share/kibana/config
func initConfigContainer(kb kbv1.Kibana) corev1.Container {
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
			ConfigVolume(kb).VolumeMount(),
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
