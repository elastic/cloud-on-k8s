// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package volume

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

var downwardApiVolume = corev1.Volume{
	Name: volume.DownwardApiVolumeName,
	VolumeSource: corev1.VolumeSource{
		DownwardAPI: &corev1.DownwardAPIVolumeSource{
			Items: []corev1.DownwardAPIVolumeFile{
				{
					Path: volume.LabelsFile,
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.labels",
					},
				},
			},
		},
	},
}

var downwardApiVolumeMount = corev1.VolumeMount{
	Name:      volume.DownwardApiVolumeName,
	MountPath: volume.DownwardApiMountPath,
	ReadOnly:  true,
}

type DownwardApi struct{}

func (DownwardApi) Name() string {
	return volume.DownwardApiVolumeName
}

func (DownwardApi) Volume() corev1.Volume {
	return downwardApiVolume
}

func (DownwardApi) VolumeMount() corev1.VolumeMount {
	return downwardApiVolumeMount
}
