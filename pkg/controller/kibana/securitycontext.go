package kibana

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

var (
	defaultSecurityContext = corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(bool(false)),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				corev1.Capability("ALL"),
			},
		},
		Privileged:             ptr.To(bool(false)),
		ReadOnlyRootFilesystem: ptr.To(bool(true)),
		RunAsUser:              ptr.To(int64(1000)),
		RunAsGroup:             ptr.To(int64(1000)),
	}
	defaultPodSecurityContext = corev1.PodSecurityContext{
		FSGroup: ptr.To(int64(1000)),
	}
)
