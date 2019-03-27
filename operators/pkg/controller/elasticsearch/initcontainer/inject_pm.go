// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	envBinDirectoryPath = "LOCAL_BIN"
	script              = `
		#!/usr/bin/env bash -eu
		cp process-manager $` + envBinDirectoryPath + `
`
)

var ExtraBinaries = SharedVolume{
	Name:                   "local-bin-volume",
	InitContainerMountPath: "/volume/bin",
	EsContainerMountPath:   volume.ExtraBinariesPath,
}

func NewInjectProcessManagerInitContainer(imageName string) (corev1.Container, error) {
	container := corev1.Container{
		Image: imageName,
		Env: []corev1.EnvVar{
			{Name: envBinDirectoryPath, Value: ExtraBinaries.InitContainerMountPath},
		},
		Name:         "inject-process-manager",
		Command:      []string{"bash", "-c", script},
		VolumeMounts: []corev1.VolumeMount{ExtraBinaries.InitContainerVolumeMount()},
	}
	return container, nil
}
