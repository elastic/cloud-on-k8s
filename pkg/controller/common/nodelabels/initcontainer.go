// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodelabels

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	corev1 "k8s.io/api/core/v1"

	commonvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	esvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
)

// WaitForAnnotationsContainerName is the name of the init container that blocks Pod start
// until the operator has patched the expected node-derived annotations onto the Pod.
const WaitForAnnotationsContainerName = "elastic-internal-wait-for-node-labels"

var waitScriptTemplate = template.Must(template.New("").Parse(
	`#!/usr/bin/env bash
set -eu
expected_annotations=({{ .ExpectedAnnotations }})
annotations_file={{ .AnnotationsFile }}
function annotations_exist() {
  for expected_annotation in "${expected_annotations[@]}"; do
    if ! grep -qE "^${expected_annotation}=" "${annotations_file}"; then
      return 1
    fi
  done
  return 0
}
echo "Waiting for the following annotations to be set on Pod: {{ .ExpectedAnnotations }}"
while ! annotations_exist; do sleep 2; done
echo "All expected annotations are set."
`))

// DownwardAPIVolume returns the downward API volume that exposes the Pod annotations file
// under the path polled by the wait-for-annotations init container.
func DownwardAPIVolume() commonvolume.DownwardAPI {
	return commonvolume.DownwardAPI{}.WithAnnotations(true)
}

// WaitForAnnotationsInitContainer builds an init container that blocks until the operator
// has patched all expectedAnnotations onto the Pod's metadata.annotations. This mirrors the
// behavior of the Elasticsearch prepare-fs init container and prevents the main container
// from starting while the downward-API annotations file is still missing labels that users
// may consume via `valueFrom.fieldRef`.
//
// The image and resources are left unset so they are inherited from the main container via
// PodTemplateBuilder.WithInitContainerDefaults. Callers must also add the volume returned
// by DownwardAPIVolume to the Pod.
func WaitForAnnotationsInitContainer(expectedAnnotations []string) (corev1.Container, error) {
	buf := bytes.Buffer{}
	if err := waitScriptTemplate.Execute(&buf, struct {
		ExpectedAnnotations string
		AnnotationsFile     string
	}{
		ExpectedAnnotations: strings.Join(expectedAnnotations, " "),
		AnnotationsFile:     fmt.Sprintf("%s/%s", esvolume.DownwardAPIMountPath, esvolume.AnnotationsFile),
	}); err != nil {
		return corev1.Container{}, err
	}
	return corev1.Container{
		Name:    WaitForAnnotationsContainerName,
		Command: []string{"bash", "-c", buf.String()},
		VolumeMounts: []corev1.VolumeMount{
			DownwardAPIVolume().VolumeMount(),
		},
	}, nil
}
