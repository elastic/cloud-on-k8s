// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filebeat

import (
	"fmt"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	Type common.Type = "filebeat"

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
	common.DriverParams
	common.Driver
}

func NewDriver(params common.DriverParams) common.Driver {
	return &Driver{DriverParams: params}
}

func (d *Driver) Reconcile() *reconciler.Results {
	preset := d.fromPreset(d.Beat.Spec.Preset)
	return common.Reconcile(
		container.FilebeatImage,
		d.DriverParams,
		preset)
}

func (d *Driver) fromPreset(name beatv1beta1.PresetName) common.Preset {
	switch name {
	case beatv1beta1.FilebeatK8sAutodiscoverPreset:
		// default to DaemonSet
		if d.Beat.Spec.DaemonSet == nil {
			d.Beat.Spec.DaemonSet = &beatv1beta1.DaemonSetSpec{}
		}

		return common.Preset{
			Config:    k8sAutodiscoverPresetConfig,
			RoleNames: []string{},
			PodTemplateFunc: func(nsName types.NamespacedName, builder *defaults.PodTemplateBuilder) {
				for _, v := range []volume.VolumeLike{
					volume.NewReadOnlyHostVolume(
						HostContainersVolumeName,
						HostContainersPath,
						HostContainersMountPath),
					volume.NewReadOnlyHostVolume(
						HostContainersLogsVolumeName,
						HostContainersLogsPath,
						HostContainersLogsMountPath),
					volume.NewReadOnlyHostVolume(
						HostPodsLogsVolumeName,
						HostPodsLogsPath,
						HostPodsLogsMountPath),
					volume.NewSecretVolume(
						common.ConfigSecretName(string(Type), nsName.Name),
						common.ConfigVolumeName,
						common.ConfigMountPath,
						common.ConfigFileName,
						0600),
					volume.NewHostVolume(
						common.DataVolumeName,
						fmt.Sprintf(common.DataMountPathTemplate, nsName.Namespace, nsName.Name, Type),
						fmt.Sprintf(common.DataPathTemplate, Type),
						false,
						corev1.HostPathDirectoryOrCreate),
				} {
					builder.WithVolumes(v.Volume()).WithVolumeMounts(v.VolumeMount())
				}

				builder.
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
