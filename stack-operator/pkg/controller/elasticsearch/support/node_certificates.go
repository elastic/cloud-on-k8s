package support

import (
	"fmt"
	"strings"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	corev1 "k8s.io/api/core/v1"
)

// ConfigureNodeCertificates configures node certificates for the provided pod
func ConfigureNodeCertificates(pod corev1.Pod) corev1.Pod {
	nodeCertificatesVolume := NewSecretVolumeWithMountPath(
		nodecerts.NodeCertificateSecretObjectKeyForPod(pod).Name,
		"node-certificates",
		"/usr/share/elasticsearch/config/node-certs",
	)
	podSpec := pod.Spec

	podSpec.Volumes = append(podSpec.Volumes, nodeCertificatesVolume.Volume())
	for i, container := range podSpec.InitContainers {
		podSpec.InitContainers[i].VolumeMounts =
			append(container.VolumeMounts, nodeCertificatesVolume.VolumeMount())
	}
	for i, container := range podSpec.Containers {
		podSpec.Containers[i].VolumeMounts = append(container.VolumeMounts, nodeCertificatesVolume.VolumeMount())

		for _, proto := range []string{"http", "transport"} {
			podSpec.Containers[i].Env = append(podSpec.Containers[i].Env,
				corev1.EnvVar{
					Name:  fmt.Sprintf("xpack.security.%s.ssl.enabled", proto),
					Value: "true",
				},
				corev1.EnvVar{
					Name:  fmt.Sprintf("xpack.security.%s.ssl.key", proto),
					Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "node.key"}, "/"),
				},
				corev1.EnvVar{
					Name:  fmt.Sprintf("xpack.security.%s.ssl.certificate", proto),
					Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "cert.pem"}, "/"),
				},
				corev1.EnvVar{
					Name:  fmt.Sprintf("xpack.security.%s.ssl.certificate_authorities", proto),
					Value: strings.Join([]string{nodeCertificatesVolume.VolumeMount().MountPath, "ca.pem"}, "/"),
				},
			)
		}

		podSpec.Containers[i].Env = append(podSpec.Containers[i].Env,
			corev1.EnvVar{
				Name:  "xpack.security.transport.ssl.verification_mode",
				Value: "certificate",
			},
			corev1.EnvVar{Name: "READINESS_PROBE_PROTOCOL", Value: "https"},

			// client profiles
			corev1.EnvVar{Name: "transport.profiles.client.xpack.security.type", Value: "client"},
			corev1.EnvVar{Name: "transport.profiles.client.xpack.security.ssl.client_authentication", Value: "none"},
		)

	}
	pod.Spec = podSpec

	return pod
}
