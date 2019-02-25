// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	// CertInitializerExecutable is the absolute path to the cert-initializer executable in the container.
	CertInitializerExecutable = "/root/cert-initializer"
	// CertInitializerPort to use for CSR http requests from the operator to the cert-initializer init container.
	CertInitializerPort = 8001
	// CertInitializerContainerName is the name of the init container in the pod.
	CertInitializerContainerName = "cert-initializer"

	// PrivateKeyFileName is the name of the private key file inside the PrivateKeySharedVolume.
	PrivateKeyFileName = "node.key"
)

// PrivateKeySharedVolume shares the private key across cert-initializer and ES containers.
var PrivateKeySharedVolume = SharedVolume{
	Name:                   "private-key-volume",
	InitContainerMountPath: "/mnt/elastic/private-key",
	EsContainerMountPath:   "/usr/share/elasticsearch/config/private-key",
}

// NewCertInitializerContainer creates an init container to handle TLS cert initialization,
// by reusing an executable provided in the given image.
// See cmd/cert-initializer/README.md for more details.
func NewCertInitializerContainer(imageName string, nodeCertificatesVolume volume.SecretVolume) (corev1.Container, error) {
	privileged := false
	initContainerRunAsUser := defaultInitContainerRunAsUser
	container := corev1.Container{
		Image:           imageName,
		ImagePullPolicy: corev1.PullAlways,
		Name:            CertInitializerContainerName,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
			RunAsUser:  &initContainerRunAsUser,
		},
		Ports: []corev1.ContainerPort{
			{Name: "csr", ContainerPort: int32(CertInitializerPort), Protocol: corev1.ProtocolTCP},
		},
		Command: []string{CertInitializerExecutable},
		VolumeMounts: []corev1.VolumeMount{
			// access node-certs that will also be mounted in ES container
			nodeCertificatesVolume.VolumeMount(),
			// create (or reuse) private key for the ES container
			PrivateKeySharedVolume.InitContainerVolumeMount(),
		},
	}
	return container, nil
}
