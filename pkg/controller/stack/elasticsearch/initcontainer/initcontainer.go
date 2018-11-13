package initcontainer

import (
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/keystore"
	corev1 "k8s.io/api/core/v1"
)

// defaultInitContainerRunAsUser is the user id the init container should run as
const defaultInitContainerRunAsUser int64 = 0

// NewInitContainers creates init containers according to the given parameters
func NewInitContainers(imageName string, linkedFiles LinkedFilesArray, keystoreSettings []keystore.Setting, SetVMMaxMapCount bool) ([]corev1.Container, error) {
	containers := []corev1.Container{}
	if SetVMMaxMapCount {
		// Only create the privileged init container if needed
		osSettingsContainer, err := NewOSSettingsInitContainer(imageName)
		if err != nil {
			return nil, err
		}
		containers = append(containers, osSettingsContainer)
	}
	prepareFsContainer, err := NewPrepareFSInitContainer(imageName, linkedFiles, keystoreSettings)
	if err != nil {
		return nil, err
	}
	containers = append(containers, prepareFsContainer)
	return containers, nil
}
