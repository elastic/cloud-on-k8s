// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filebeat

import (
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	beatcommon "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
)

const (
	Type beatcommon.Type = "filebeat"

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
	beatcommon.DriverParams
	beatcommon.Driver
}

func NewDriver(params beatcommon.DriverParams) beatcommon.Driver {
	spec := &params.Beat.Spec
	// use the default for filebeat type if not provided
	if spec.DaemonSet == nil && spec.Deployment == nil {
		spec.DaemonSet = &beatv1beta1.DaemonSetSpec{}
	}

	return &Driver{DriverParams: params}
}

func (d *Driver) Reconcile() *reconciler.Results {
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

	defaultConfigs, err := d.configParams()
	if err != nil {
		return reconciler.NewResult(d.DriverParams.Context).WithError(err)
	}

	return beatcommon.Reconcile(
		d.DriverParams,
		defaultConfigs,
		container.FilebeatImage,
		f)
}
