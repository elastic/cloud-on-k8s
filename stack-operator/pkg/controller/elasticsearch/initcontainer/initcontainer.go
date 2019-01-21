package initcontainer

import (
	corev1 "k8s.io/api/core/v1"
)

// defaultInitContainerRunAsUser is the user id the init container should run as
const defaultInitContainerRunAsUser int64 = 0

// NewInitContainers creates init containers according to the given parameters
func NewInitContainers(
	imageName string,
	linkedFiles LinkedFilesArray,
	SetVMMaxMapCount bool,
	additional ...corev1.Container,
) ([]corev1.Container, error) {
	var containers []corev1.Container
	if SetVMMaxMapCount {
		// Only create the privileged init container if needed
		osSettingsContainer, err := NewOSSettingsInitContainer(imageName)
		if err != nil {
			return nil, err
		}
		containers = append(containers, osSettingsContainer)
	}
	prepareFsContainer, err := NewPrepareFSInitContainer(imageName, linkedFiles)
	if err != nil {
		return nil, err
	}
	containers = append(containers, prepareFsContainer)
	containers = append(containers, additional...)
	return containers, nil
}
