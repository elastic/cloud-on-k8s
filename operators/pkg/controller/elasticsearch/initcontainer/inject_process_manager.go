// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	envBinDirectoryPath = "LOCAL_BIN"
	script              = `
		#!/usr/bin/env bash -eu
		cp process-manager $` + envBinDirectoryPath + `
`
)

var ProcessManagerVolume = SharedVolume{
	Name:                   "elastic-internal-process-manager",
	InitContainerMountPath: volume.ProcessManagerEmptyDirMountPath,
	EsContainerMountPath:   volume.ProcessManagerEmptyDirMountPath,
}

func NewInjectProcessManagerInitContainer(imageName string) (corev1.Container, error) {
	container := corev1.Container{
		Image: imageName,
		Env: []corev1.EnvVar{
			{Name: envBinDirectoryPath, Value: ProcessManagerVolume.InitContainerMountPath},
		},
		Name:         injectProcessManagerContainerName,
		Command:      []string{"bash", "-c", script},
		VolumeMounts: []corev1.VolumeMount{ProcessManagerVolume.InitContainerVolumeMount()},
	}
	return container, nil
}
