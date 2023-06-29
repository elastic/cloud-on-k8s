// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import corev1 "k8s.io/api/core/v1"

var (
	defaultOptional = false
)

type VolumeLike interface { //nolint:revive
	Name() string
	Volume() corev1.Volume
	VolumeMount() corev1.VolumeMount
}

func Resolve(volumeLikes []VolumeLike) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := make([]corev1.Volume, len(volumeLikes))
	volumeMounts := make([]corev1.VolumeMount, len(volumeLikes))
	for i := range volumeLikes {
		volumes[i] = volumeLikes[i].Volume()
		volumeMounts[i] = volumeLikes[i].VolumeMount()
	}
	return volumes, volumeMounts
}
