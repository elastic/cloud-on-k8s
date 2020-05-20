// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filebeat

import (
	"fmt"

	commonbeat "github.com/elastic/cloud-on-k8s/pkg/controller/common/beat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	Type commonbeat.Type = "filebeat"

	HostContainersVolumeName = "varlibdockercontainers"
	HostContainersPath       = "/var/lib/docker/containers"
	HostContainersMountPath  = "/var/lib/docker/containers"

	HostContainersLogsVolumeName = "varlogcontainers"
	HostContainersLogsPath       = "/var/log/containers"
	HostContainersLogsMountPath  = "/var/log/containers"

	HostPodsLogsVolumeName = "varlogpods"
	HostPodsLogsPath       = "/var/log/pods"
	HostPodsLogsMountPath  = "/var/log/pods"

	HostFilebeatDataVolumeName   = "data"
	HostFilebeatDataPathTemplate = "/var/lib/%s/%s/filebeat-data"
	HostFilebeatDataMountPath    = "/usr/share/filebeat/data"
)

type Driver struct {
	commonbeat.DriverParams
	commonbeat.Driver
}

func NewDriver(params commonbeat.DriverParams) commonbeat.Driver {
	return &Driver{DriverParams: params}
}

func (d *Driver) Reconcile() commonbeat.DriverResults {
	f := func(builder *defaults.PodTemplateBuilder) {
		containersVolume := volume.NewReadOnlyHostVolume(HostContainersVolumeName, HostContainersPath, HostContainersMountPath)
		containersLogsVolume := volume.NewReadOnlyHostVolume(HostContainersLogsVolumeName, HostContainersLogsPath, HostContainersLogsMountPath)
		podsLogsVolume := volume.NewReadOnlyHostVolume(HostPodsLogsVolumeName, HostPodsLogsPath, HostPodsLogsMountPath)
		hostFilebeatDataPath := fmt.Sprintf(HostFilebeatDataPathTemplate, d.Owner.GetNamespace(), d.Owner.GetName())
		filebeatDataVolume := volume.NewHostVolume(
			HostFilebeatDataVolumeName,
			hostFilebeatDataPath,
			HostFilebeatDataMountPath,
			false,
			corev1.HostPathDirectoryOrCreate)

		for _, volume := range []volume.VolumeLike{
			containersVolume,
			containersLogsVolume,
			podsLogsVolume,
			filebeatDataVolume,
		} {
			builder.WithVolumes(volume.Volume()).WithVolumeMounts(volume.VolumeMount())
		}
	}

	if d.DaemonSet == nil && d.Deployment == nil {
		d.DaemonSet = &commonbeat.DaemonSetSpec{}
	}

	return commonbeat.Reconcile(
		d.DriverParams,
		defaultConfig,
		container.FilebeatImage,
		f)
}
