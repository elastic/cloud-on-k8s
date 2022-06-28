// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"fmt"
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	esvolume "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
)

const (
	SuspendScriptConfigKey = "suspend.sh"
	SuspendedHostsFile     = "suspended_pods.txt"
)

var SuspendScript = fmt.Sprintf(`#!/usr/bin/env bash
set -eu

while [[ $(grep -Exc $HOSTNAME /mnt/elastic-internal/scripts/%s) -eq 1 ]]; do
echo Pod suspended via %s annotation
sleep 10
done
`, SuspendedHostsFile, esv1.SuspendAnnotation)

// RenderSuspendConfiguration renders the configuration used by the SuspendScript.
func RenderSuspendConfiguration(es esv1.Elasticsearch) string {
	names := es.SuspendedPodNames().AsSlice()
	names.Sort()
	return strings.Join(names, "\n")
}

// NewSuspendInitContainer creates an init container to run the script to check for suspended Pods.
func NewSuspendInitContainer() corev1.Container {
	return corev1.Container{
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            SuspendContainerName,
		Env:             defaults.PodDownwardEnvVars(),
		Command:         []string{"bash", "-c", path.Join(esvolume.ScriptsVolumeMountPath, SuspendScriptConfigKey)},
	}
}
