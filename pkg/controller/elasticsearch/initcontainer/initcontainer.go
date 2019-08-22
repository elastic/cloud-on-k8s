// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	corev1 "k8s.io/api/core/v1"
)

// defaultInitContainerRunAsUser is the user id the init container should run as
const defaultInitContainerRunAsUser int64 = 0

const (
	// osSettingsContainerName is the name of the container that tweaks os-level settings
	osSettingsContainerName = "elastic-internal-init-os-settings"
	// prepareFilesystemContainerName is the name of the container that prepares the filesystem
	PrepareFilesystemContainerName = "elastic-internal-init-filesystem"
)

// NewInitContainers creates init containers according to the given parameters
func NewInitContainers(
	elasticsearchImage string,
	setVMMaxMapCount *bool,
	transportCertificatesVolume volume.SecretVolume,
	clusterName string,
	keystoreResources *keystore.Resources,
) ([]corev1.Container, error) {
	var containers []corev1.Container
	// create the privileged init container if not explicitly disabled by the user
	if setVMMaxMapCount == nil || *setVMMaxMapCount {
		osSettingsContainer, err := NewOSSettingsInitContainer(elasticsearchImage)
		if err != nil {
			return nil, err
		}
		containers = append(containers, osSettingsContainer)
	}
	prepareFsContainer, err := NewPrepareFSInitContainer(elasticsearchImage, transportCertificatesVolume, clusterName)
	if err != nil {
		return nil, err
	}
	containers = append(containers, prepareFsContainer)

	if keystoreResources != nil {
		containers = append(containers, keystoreResources.InitContainer)
	}

	return containers, nil
}
