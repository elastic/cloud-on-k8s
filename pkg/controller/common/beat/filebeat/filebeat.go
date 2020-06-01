// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filebeat

import (
	commonbeat "github.com/elastic/cloud-on-k8s/pkg/controller/common/beat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
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
)

type Driver struct {
	commonbeat.DriverParams
	commonbeat.Driver
}

func NewDriver(params commonbeat.DriverParams) commonbeat.Driver {
	// use the default for filebeat type if not provided
	if params.DaemonSet == nil && params.Deployment == nil {
		params.DaemonSet = &commonbeat.DaemonSetSpec{}
	}

	return &Driver{DriverParams: params}
}

func (d *Driver) Reconcile() (*commonbeat.DriverStatus, *reconciler.Results) {
	f := func(builder *defaults.PodTemplateBuilder) {
		containersVolume := volume.NewReadOnlyHostVolume(HostContainersVolumeName, HostContainersPath, HostContainersMountPath)
		containersLogsVolume := volume.NewReadOnlyHostVolume(HostContainersLogsVolumeName, HostContainersLogsPath, HostContainersLogsMountPath)
		podsLogsVolume := volume.NewReadOnlyHostVolume(HostPodsLogsVolumeName, HostPodsLogsPath, HostPodsLogsMountPath)

		for _, volume := range []volume.VolumeLike{
			containersVolume,
			containersLogsVolume,
			podsLogsVolume,
		} {
			builder.WithVolumes(volume.Volume()).WithVolumeMounts(volume.VolumeMount())
		}
	}

	return commonbeat.Reconcile(
		d.DriverParams,
		defaultConfig,
		container.FilebeatImage,
		f)
}
