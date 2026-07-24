// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodelabels

import (
	"errors"

	corev1 "k8s.io/api/core/v1"

	commonvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
)

// WaitForAnnotationsContainerName is the name of the init container that blocks Pod start
// until the operator has patched the expected node-derived annotations onto the Pod.
const WaitForAnnotationsContainerName = "elastic-internal-wait-for-node-labels"

// DownwardAPIVolume returns the downward API volume that exposes the Pod annotations file
// under the path polled by the wait-for-annotations init container.
func DownwardAPIVolume() commonvolume.DownwardAPI {
	return commonvolume.DownwardAPI{}.WithAnnotations(true)
}

// WaitForAnnotationsInitContainer builds an init container that blocks until the operator
// has patched all expectedAnnotations onto the Pod's metadata.annotations. It runs the
// operator binary's "wait-for-annotations" subcommand using the operator's own image,
// which removes any dependency on the stack/component image having a shell or grep.
//
// operatorImage must be non-empty; an error is returned otherwise so callers fail loudly
// rather than silently falling back to the stack image via PodTemplateBuilder.WithInitContainerDefaults.
// Callers must also add the volume returned by DownwardAPIVolume to the Pod.
func WaitForAnnotationsInitContainer(operatorImage string, expectedAnnotations []string) (corev1.Container, error) {
	if operatorImage == "" {
		return corev1.Container{}, errors.New("operator image is required to build the wait-for-annotations init container; " +
			"set --operator-image or ensure the operator can introspect its own Pod")
	}

	cmd := []string{
		"/elastic-operator",
		"wait-for-annotations",
		"--file=" + DownwardAPIVolume().AnnotationsFilePath(),
	}
	for _, a := range expectedAnnotations {
		cmd = append(cmd, "--annotation="+a)
	}

	return corev1.Container{
		Name:    WaitForAnnotationsContainerName,
		Image:   operatorImage,
		Command: cmd,
		VolumeMounts: []corev1.VolumeMount{
			DownwardAPIVolume().VolumeMount(),
		},
	}, nil
}
