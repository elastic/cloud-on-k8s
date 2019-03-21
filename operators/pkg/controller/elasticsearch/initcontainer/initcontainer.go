// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

// defaultInitContainerRunAsUser is the user id the init container should run as
const defaultInitContainerRunAsUser int64 = 0

// NewInitContainers creates init containers according to the given parameters
func NewInitContainers(
	elasticsearchImage string,
	operatorImage string,
	linkedFiles LinkedFilesArray,
	SetVMMaxMapCount bool,
	nodeCertificatesVolume volume.SecretVolume,
	additional ...corev1.Container,
) ([]corev1.Container, error) {
	var containers []corev1.Container
	if SetVMMaxMapCount {
		// Only create the privileged init container if needed
		osSettingsContainer, err := NewOSSettingsInitContainer(elasticsearchImage)
		if err != nil {
			return nil, err
		}
		containers = append(containers, osSettingsContainer)
	}
	prepareFsContainer, err := NewPrepareFSInitContainer(elasticsearchImage, linkedFiles)
	if err != nil {
		return nil, err
	}

	certInitializerContainer, err := NewCertInitializerContainer(operatorImage, nodeCertificatesVolume)
	if err != nil {
		return nil, err
	}

	injectProcessManager, err := NewInjectProcessManagerInitContainer(operatorImage, EsBinSharedVolume)
	if err != nil {
		return nil, err
	}

	containers = append(containers, prepareFsContainer, injectProcessManager, certInitializerContainer)
	containers = append(containers, additional...)
	return containers, nil
}
