// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package volume

import corev1 "k8s.io/api/core/v1"

var (
	defaultOptional = false
)

type VolumeLike interface {
	Name() string
	Volume() corev1.Volume
	VolumeMount() corev1.VolumeMount
}
