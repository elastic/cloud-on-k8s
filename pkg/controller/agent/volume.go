// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/blang/semver/v4"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	container "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/pointer"
)

const (
	hostPathVolumeInitContainerName = "permissions"
)

var (
	hostPathVolumeInitContainerCommand = []string{
		"/usr/bin/env",
		"bash",
		"-c",
		permissionsScript(),
	}
	hostPathVolumeInitContainerResources = corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("128Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("128Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
	}
)

// if volume doesn't containe emptydir
// and Version is above 7.15
// and not running as root....
// TODO WTF ABOUT OPENSHIFT?
// add an initContainer that does
func maybeAgentInitContainerForHostpathVolume(spec *agentv1alpha1.AgentSpec, v semver.Version) (initContainers []corev1.Container) {
	// Only add initContainer to chown hostpath data volume for Agent > 7.15
	if !v.GTE(version.MinFor(7, 15, 0)) {
		return nil
	}

	image := spec.Image
	if image == "" {
		image = container.ImageRepository(container.AgentImage, spec.Version)
	}

	if !dataVolumeIsEmptyDir(spec) && !runningAsRoot(spec) {
		initContainers = append(initContainers, corev1.Container{
			Image:   image,
			Command: hostPathVolumeInitContainerCommand,
			Name:    hostPathVolumeInitContainerName,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: pointer.Int64(0),
			},
			Resources: hostPathVolumeInitContainerResources,
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      DataVolumeName,
					MountPath: DataMountPath,
				},
			},
		})
	}

	return initContainers
}

func permissionsScript() string {
	return `#!/usr/bin/env bash
set -e
find /usr/share/elastic-agent -ls
if [[ -d /usr/share/elastic-agent/state ]]; then
  echo "Adjusting g+rw of /usr/share/elastic-agent/state"
  chmod g+rw /usr/share/elastic-agent/state
  echo "Adjusting group ownership of /usr/share/elastic-agent/state"
  chgrp 1000 /usr/share/elastic-agent/state
  if [ -n "$(ls -A /usr/share/elastic-agent/state 2>/dev/null)" ]; then
    echo "Adjusting group ownership of /usr/share/elastic-agent/state/*"
    chgrp 1000 /usr/share/elastic-agent/state/*
    echo "Adjusting g+rw of /usr/share/elastic-agent/state/*"
    chmod g+rw /usr/share/elastic-agent/state/*
  fi
fi
`
}

// Wonder if this is right in openshift?
func runningAsRoot(spec *agentv1alpha1.AgentSpec) bool {
	if spec.DaemonSet != nil {
		templateSpec := spec.DaemonSet.PodTemplate.Spec
		if templateSpec.SecurityContext != nil &&
			templateSpec.SecurityContext.RunAsUser != nil && *templateSpec.SecurityContext.RunAsUser == 0 {
			return true
		}
		return containerRunningAsUser0(templateSpec)
	}
	if spec.Deployment != nil {
		templateSpec := spec.Deployment.PodTemplate.Spec
		if templateSpec.SecurityContext != nil &&
			templateSpec.SecurityContext.RunAsUser != nil && *templateSpec.SecurityContext.RunAsUser == 0 {
			return true
		}
		return containerRunningAsUser0(templateSpec)
	}
	return false
}

func containerRunningAsUser0(spec corev1.PodSpec) bool {
	for _, container := range spec.Containers {
		if container.Name == "agent" {
			if container.SecurityContext == nil {
				return false
			}
			if container.SecurityContext.RunAsUser != nil && *container.SecurityContext.RunAsUser == 0 {
				return true
			}
			return false
		}
	}
	return false
}

func dataVolumeIsEmptyDir(spec *agentv1alpha1.AgentSpec) bool {
	if spec.DaemonSet != nil {
		return volumeIsEmptyDir(spec.DaemonSet.PodTemplate.Spec.Volumes)
	}
	if spec.Deployment != nil {
		return volumeIsEmptyDir(spec.Deployment.PodTemplate.Spec.Volumes)
	}
	return false
}

func volumeIsEmptyDir(vols []corev1.Volume) bool {
	for _, vol := range vols {
		if vol.Name == DataVolumeName && vol.VolumeSource.EmptyDir != nil {
			return true
		}
	}
	return false
}
