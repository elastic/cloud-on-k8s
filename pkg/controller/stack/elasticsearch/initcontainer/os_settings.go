package initcontainer

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// VMMaxMapCount maps settings recommended in
// https://www.elastic.co/guide/en/elasticsearch/reference/current/docker.html#docker-cli-run-prod-mode
const VMMaxMapCount = 262144

// NewOSSettingsInitContainer creates an init container to handle OS settings tweaks
// It needs to be privileged.
func NewOSSettingsInitContainer(imageName string) (corev1.Container, error) {
	privileged := true
	initContainerRunAsUser := defaultInitContainerRunAsUser
	container := corev1.Container{
		Image:           imageName,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            "tweak-os-settings",
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
			RunAsUser:  &initContainerRunAsUser,
		},
		Command:      []string{"sysctl", "-w", fmt.Sprintf("vm.max_map_count=%d", 262144)},
		VolumeMounts: SharedVolumes.InitContainerVolumeMounts(),
	}
	return container, nil
}
