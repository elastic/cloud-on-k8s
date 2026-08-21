// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

const (
	DownwardAPIVolumeName = "downward-api"
	DownwardAPIMountPath  = "/mnt/elastic-internal/downward-api"
	LabelsFile            = "labels"
	AnnotationsFile       = "annotations"
)

var downwardAPIVolumeMount = corev1.VolumeMount{
	Name:      DownwardAPIVolumeName,
	MountPath: DownwardAPIMountPath,
	ReadOnly:  true,
}

type DownwardAPI struct {
	withAnnotations bool
}

// WithAnnotations defines if the metadata.annotations must be available in the downward API volume.
func (d DownwardAPI) WithAnnotations(withAnnotations bool) DownwardAPI {
	d.withAnnotations = withAnnotations
	return d
}

var _ VolumeLike = DownwardAPI{}

func (DownwardAPI) Name() string {
	return DownwardAPIVolumeName
}

func (d DownwardAPI) Volume() corev1.Volume {
	downwardAPIVolume := corev1.Volume{
		Name: DownwardAPIVolumeName,
		VolumeSource: corev1.VolumeSource{
			DownwardAPI: &corev1.DownwardAPIVolumeSource{
				Items: []corev1.DownwardAPIVolumeFile{
					{
						Path: LabelsFile,
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.labels",
						},
					},
				},
			},
		},
	}
	if d.withAnnotations {
		downwardAPIVolume.VolumeSource.DownwardAPI.Items = append(
			downwardAPIVolume.VolumeSource.DownwardAPI.Items,
			corev1.DownwardAPIVolumeFile{
				Path: AnnotationsFile,
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.annotations",
				},
			},
		)
	}
	return downwardAPIVolume
}

func (DownwardAPI) VolumeMount() corev1.VolumeMount {
	return downwardAPIVolumeMount
}

// AnnotationsFilePath returns the absolute path of the file exposing the Pod annotations within the
// downward API volume mount. It is only populated when the volume is built WithAnnotations(true).
func (DownwardAPI) AnnotationsFilePath() string {
	return fmt.Sprintf("%s/%s", DownwardAPIMountPath, AnnotationsFile)
}
