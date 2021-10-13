// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"fmt"
	"path"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	esvolume "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	SuspendScriptConfigKey = "suspend.sh"
	SuspendedHostsFile     = "suspended_hosts.txt"
)

var SuspendScript = fmt.Sprintf(`#!/usr/bin/env bash
set -eu

while [[ $(grep -Ec $HOSTNAME /mnt/elastic-internal/scripts/%s) -eq 1 ]]; do
echo Pod suspended via %s annotation
sleep 10
done
`, SuspendedHostsFile, esv1.SuspendAnnotation)

var suspendContainerResources = corev1.ResourceRequirements{
	Requests: map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceMemory: resource.MustParse("100Mi"),
	},
	Limits: map[corev1.ResourceName]resource.Quantity{
		// Memory limit should be at least 12582912 when running with CRI-O
		// Less than 100Mi and Elasticsearch tools like elasticsearch-node run into OOM
		corev1.ResourceMemory: resource.MustParse("100Mi"),
	},
}

func NewSuspendInitContainer() corev1.Container {
	return corev1.Container{
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            SuspendContainerName,
		Env:             defaults.PodDownwardEnvVars(),
		Command:         []string{"bash", "-c", path.Join(esvolume.ScriptsVolumeMountPath, SuspendScriptConfigKey)},
		Resources:       suspendContainerResources,
	}
}
