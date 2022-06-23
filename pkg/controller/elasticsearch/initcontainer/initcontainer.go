// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
)

const (
	// PrepareFilesystemContainerName is the name of the container that prepares the filesystem
	PrepareFilesystemContainerName = "elastic-internal-init-filesystem"
	// SuspendContainerName is the name of the container that is used to suspend Elasticsearch if requested by the user.
	SuspendContainerName = "elastic-internal-suspend"
)

// NewInitContainers creates init containers according to the given parameters
func NewInitContainers(
	transportCertificatesVolume volume.SecretVolume,
	keystoreResources *keystore.Resources,
	nodeLabelsAsAnnotations []string,
) ([]corev1.Container, error) {
	var containers []corev1.Container
	prepareFsContainer, err := NewPrepareFSInitContainer(transportCertificatesVolume, nodeLabelsAsAnnotations)
	if err != nil {
		return nil, err
	}
	containers = append(containers, prepareFsContainer)

	if keystoreResources != nil {
		containers = append(containers, keystoreResources.InitContainer)
	}

	containers = append(containers, NewSuspendInitContainer())

	return containers, nil
}
