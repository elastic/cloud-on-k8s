// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package volume

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

var downwardAPIVolume = corev1.Volume{
	Name: volume.DownwardAPIVolumeName,
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

var downwardAPIVolumeMount = corev1.VolumeMount{
	Name:      volume.DownwardAPIVolumeName,
	MountPath: volume.DownwardAPIMountPath,
	ReadOnly:  true,
}

type DownwardAPI struct{}

var _ VolumeLike = DownwardAPI{}

func (DownwardAPI) Name() string {
	return volume.DownwardAPIVolumeName
}

func (DownwardAPI) Volume() corev1.Volume {
	return downwardAPIVolume
}

func (DownwardAPI) VolumeMount() corev1.VolumeMount {
	return downwardAPIVolumeMount
}
