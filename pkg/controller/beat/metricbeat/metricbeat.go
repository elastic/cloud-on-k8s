// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package metricbeat

import (
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	beatcommon "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	Type beatcommon.Type = "metricbeat"

	DockerSockVolumeName = "dockersock"
	DockerSockPath       = "/var/run/docker.sock"
	DockerSockMountPath  = "/var/run/docker.sock"

	ProcVolumeName = "proc"
	ProcPath       = "/proc"
	ProcMountPath  = "/hostfs/proc"

	CGroupVolumeName = "cgroup"
	CGroupPath       = "/sys/fs/cgroup"
	CGroupMountPath  = "/hostfs/sys/fs/cgroup"
)

type Driver struct {
	beatcommon.DriverParams
	beatcommon.Driver
}

func NewDriver(params beatcommon.DriverParams) beatcommon.Driver {
	spec := &params.Beat.Spec
	// use the default for metricbeat type if not provided
	if spec.DaemonSet == nil && spec.Deployment == nil {
		spec.DaemonSet = &beatv1beta1.DaemonSetSpec{}
	}

	return &Driver{DriverParams: params}
}

func (d *Driver) Reconcile() *reconciler.Results {
	f := func(builder *defaults.PodTemplateBuilder) {
		dockerSockVolume := volume.NewHostVolume(DockerSockVolumeName, DockerSockPath, DockerSockMountPath, false, corev1.HostPathUnset)
		procVolume := volume.NewReadOnlyHostVolume(ProcVolumeName, ProcPath, ProcMountPath)
		cgroupVolume := volume.NewReadOnlyHostVolume(CGroupVolumeName, CGroupPath, CGroupMountPath)

		for _, volume := range []volume.VolumeLike{
			dockerSockVolume,
			procVolume,
			cgroupVolume,
		} {
			builder.WithVolumes(volume.Volume()).WithVolumeMounts(volume.VolumeMount())
		}

		builder.WithArgs("-e", "-c", beatcommon.ConfigMountPath, "-system.hostfs=/hostfs")
	}

	defaultConfig, err := d.defaultConfig()
	if err != nil {
		return reconciler.NewResult(d.DriverParams.Context).WithError(err)
	}

	return beatcommon.Reconcile(
		d.DriverParams,
		defaultConfig,
		container.MetricbeatImage,
		f)
}
