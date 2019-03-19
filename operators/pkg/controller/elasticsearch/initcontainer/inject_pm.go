// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import corev1 "k8s.io/api/core/v1"

func NewInjectProcessManagerInitContainer(imageName string, sharedVolume SharedVolume) (corev1.Container, error) {
	container := corev1.Container{
		Image: imageName,
		Env: []corev1.EnvVar{
			{Name: "ES_BIN", Value: sharedVolume.InitContainerMountPath},
		},
		Name: "inject-process-manager",
		Command: []string{"bash", "-c", `
			#!/usr/bin/env bash -eu
			cp process-manager $ES_BIN
    `},
		VolumeMounts: []corev1.VolumeMount{sharedVolume.InitContainerVolumeMount()},
	}
	return container, nil
}
