// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package metricbeat

import (
	"fmt"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	common "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	Type common.Type = "metricbeat"

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
	common.DriverParams
	common.Driver
}

func NewDriver(params common.DriverParams) common.Driver {
	return &Driver{DriverParams: params}
}

func (d *Driver) Reconcile() *reconciler.Results {
	preset := d.fromPreset(d.Beat.Spec.Preset)
	return common.Reconcile(
		container.MetricbeatImage,
		d.DriverParams,
		preset)
}

func (d *Driver) fromPreset(preset beatv1beta1.PresetName) common.Preset {
	switch preset {
	case beatv1beta1.MetricbeatK8sHostsPreset:
		if d.Beat.Spec.DaemonSet == nil {
			d.Beat.Spec.DaemonSet = &beatv1beta1.DaemonSetSpec{}
		}

		return common.Preset{
			RoleNames: []string{common.MetricbeatK8sHostsPresetRole},
			Config:    k8sHostsPresetConfig,
			PodTemplateFunc: func(nsName types.NamespacedName, builder *defaults.PodTemplateBuilder) {
				for _, v := range []volume.VolumeLike{
					volume.NewHostVolume(
						DockerSockVolumeName,
						DockerSockPath,
						DockerSockMountPath,
						false,
						corev1.HostPathUnset),
					volume.NewReadOnlyHostVolume(
						ProcVolumeName,
						ProcPath,
						ProcMountPath),
					volume.NewReadOnlyHostVolume(
						CGroupVolumeName,
						CGroupPath,
						CGroupMountPath),
					volume.NewHostVolume(
						common.DataVolumeName,
						fmt.Sprintf(common.DataMountPathTemplate, nsName.Namespace, nsName.Name, Type),
						fmt.Sprintf(common.DataPathTemplate, Type),
						false,
						corev1.HostPathDirectoryOrCreate),
					volume.NewSecretVolume(
						common.ConfigSecretName(string(Type), nsName.Name),
						common.ConfigVolumeName,
						common.ConfigMountPath,
						common.ConfigFileName,
						0600),
				} {
					builder.WithVolumes(v.Volume()).WithVolumeMounts(v.VolumeMount())
				}

				builder.
					WithArgs("-e", "-c", common.ConfigMountPath, "-system.hostfs=/hostfs").
					WithTerminationGracePeriod(30).
					WithEnv(corev1.EnvVar{
						Name: "NODE_NAME",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef: &corev1.ObjectFieldSelector{
								FieldPath: "spec.nodeName",
							},
						}}).
					WithResources(common.DefaultResources).
					WithHostNetwork().
					WithDNSPolicy(corev1.DNSClusterFirstWithHostNet).
					WithPodSecurityContext(corev1.PodSecurityContext{
						RunAsUser: pointer.Int64(0),
					})
			},
		}
	default:
		return common.Preset{}
	}
}
