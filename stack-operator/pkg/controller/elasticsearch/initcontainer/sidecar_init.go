package initcontainer

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/volume"

	corev1 "k8s.io/api/core/v1"
)

var script = `
     #!/usr/bin/env bash -eu
     cp keystore-updater $SHARED_BIN
    `

func NewSidecarInitContainer(sharedVolume volume.VolumeLike, operatorImage string) corev1.Container {
	return corev1.Container{
		Name:            "sidecar-init",
		Image:           operatorImage,
		ImagePullPolicy: corev1.PullAlways,
		Env: []corev1.EnvVar{
			{Name: "SHARED_BIN", Value: sharedVolume.VolumeMount().MountPath},
		},
		Command: []string{"bash", "-c", script},
		VolumeMounts: []corev1.VolumeMount{
			sharedVolume.VolumeMount(),
		},
	}
}
