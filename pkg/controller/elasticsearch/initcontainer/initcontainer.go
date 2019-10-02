// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	corev1 "k8s.io/api/core/v1"
)

// PrepareFilesystemContainerName is the name of the container that prepares the filesystem
const PrepareFilesystemContainerName = "elastic-internal-init-filesystem"

// NewInitContainers creates init containers according to the given parameters
func NewInitContainers(
	elasticsearchImage string,
	transportCertificatesVolume volume.SecretVolume,
	clusterName string,
	keystoreResources *keystore.Resources,
) ([]corev1.Container, error) {
	var containers []corev1.Container
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
