// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	container "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/pointer"
)

const (
	hostPathVolumeInitContainerName = "permissions"
)

var (
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

// maybeBeatInitContainerForHostpathVolume will return an init container that ensures that the host
// volume's permissions are sufficient for the Beat to maintain state if the Elastic Beat
// has the following attributes:
//
// 1. Beat volume is not set to emptyDir.
// 3. Beat spec is not configured to run as root.
func maybeBeatInitContainerForHostpathVolume(beat beatv1beta1.Beat) (initContainers []corev1.Container) {
	image := beat.Spec.Image
	if image == "" {
		image = container.ImageRepository(container.AgentImage, beat.Spec.Version)
	}

	mountPath := fmt.Sprintf(DataPathTemplate, beat.Spec.Type)

	euid := getUIDForBeat(beat)

	if !dataVolumeEmptyDir(beat.Spec) && !runningAsRoot(beat) {
		initContainers = append(initContainers, corev1.Container{
			Image:   image,
			Command: hostPathVolumeInitContainerCommand(mountPath, euid),
			Name:    hostPathVolumeInitContainerName,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: pointer.Int64(0),
			},
			Resources: hostPathVolumeInitContainerResources,
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      DataVolumeName,
					MountPath: mountPath,
				},
			},
		})
	}

	return initContainers
}

func getUIDForBeat(beat beatv1beta1.Beat) int64 {
	var euid int64 = 1000
	if runningAsRoot(beat) {
		return 0
	}
	var tmpl corev1.PodSpec
	if beat.Spec.DaemonSet != nil {
		tmpl = beat.Spec.DaemonSet.PodTemplate.Spec
	}
	if beat.Spec.Deployment != nil {
		tmpl = beat.Spec.Deployment.PodTemplate.Spec
	}
	if tmpl.SecurityContext != nil && tmpl.SecurityContext.RunAsUser != nil {
		return *tmpl.SecurityContext.RunAsUser
	}
	for _, container := range tmpl.Containers {
		if container.Name == beat.Spec.Type {
			if container.SecurityContext != nil && container.SecurityContext.RunAsUser != nil {
				return *container.SecurityContext.RunAsUser
			}
		}
	}
	// WHAT ABOUT OPENSHIFT ????
	return euid
}

// hostPathVolumeInitContainerCommand returns the container command
// for maintaining permissions for Elastic Beat.
func hostPathVolumeInitContainerCommand(mountPath string, euid int64) []string {
	return []string{
		"/usr/bin/env",
		"bash",
		"-c",
		fmt.Sprintf(`#!/usr/bin/env bash
set -e
if [[ -d %[1]s ]]; then
  chmod g+rw %[1]s
  chgrp 1000 %[1]s
  if [ -n "$(ls -A %[1]s 2>/dev/null)" ]; then
    # Beat is a bit different than Agent.
	# It appears to maintain files in the root */data directory
	# plus files in subdirectories such as
	# */data/registry/filebeat/meta.json
	# hence the need for recursive operations.
    chgrp -R 1000 %[1]s/*
    chmod -R g+rw %[1]s/*
	# Beat requires files to be owned by it's UID, or it tries to update them
	# Exiting: failed to open store 'filebeat': failed to update meta file permissions:
	# chmod /usr/share/filebeat/data/registry/filebeat/meta.json: operation not permitted
    chown -R %[2]d %[1]s/*
	# Also the keystore can only be read/writable by UID
	# could not initialize the keystore: file ("/usr/share/filebeat/data/filebeat.keystore")
	# can only be writable and readable by the owner but the permissions are "-rw-rw----"
    chmod 0600 %[1]s/*.keystore
  fi
fi
`, mountPath, euid)}
}

// dataVolumeEmptyDir will return true if either the Daemonset or Deployment for
// Elastic Beats has it's Beat volume configured for EmptyDir.
func dataVolumeEmptyDir(spec beatv1beta1.BeatSpec) bool {
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
