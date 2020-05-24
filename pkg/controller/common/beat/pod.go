// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"fmt"
	"hash"

	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	commonhash "github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

const (
	CAVolumeName = "es-certs"
	CAMountPath  = "/mnt/elastic-internal/es-certs/"
	CAFileName   = "ca.crt"

	ConfigVolumeName = "config"
	ConfigMountPath  = "/etc/beat.yml"
	ConfigFileName   = "beat.yml"

	DataVolumeName        = "data"
	DataMountPathTemplate = "/var/lib/%s/%s/%s-data"
	DataPathTemplate      = "/usr/share/%s/data"

	// ConfigChecksumLabel is a label used to store a Beats config checksum.
	ConfigChecksumLabel = "beat.k8s.elastic.co/config-checksum"

	// VersionLabelName is a label used to track the version of a Beat Pod.
	VersionLabelName = "beat.k8s.elastic.co/version"
)

func buildPodTemplate(params DriverParams, defaultImage container.Image, f func(builder *defaults.PodTemplateBuilder), checksum hash.Hash) corev1.PodTemplateSpec {
	podTemplate := params.GetPodTemplate()

	// Token mounting gets defaulted to false, which prevents from detecting whether user set it.
	// Instead, checking that here, before the default is applied.
	if podTemplate.Spec.AutomountServiceAccountToken == nil {
		t := true
		podTemplate.Spec.AutomountServiceAccountToken = &t
	}

	builder := defaults.NewPodTemplateBuilder(podTemplate, params.Type)

	if f != nil {
		f(builder)
	}

	builder = builder.
		WithTerminationGracePeriod(30).
		WithEnv(corev1.EnvVar{
			Name: "NODE_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			}}).
		WithResources(defaultResources).
		WithHostNetwork().
		WithLabels(map[string]string{
			ConfigChecksumLabel: fmt.Sprintf("%x", checksum.Sum(nil)),
			VersionLabelName:    params.Version}).
		WithDockerImage(params.Image, container.ImageRepository(defaultImage, params.Version)).
		WithArgs("-e", "-c", ConfigMountPath).
		WithDNSPolicy(corev1.DNSClusterFirstWithHostNet).
		WithSecurityContext(corev1.SecurityContext{
			RunAsUser: pointer.Int64(0),
		})

	if ShouldSetupAutodiscoverRBAC() {
		autodiscoverServiceAccountName := ServiceAccountName(params.Owner.GetName())
		// If SA is already provided, the call will be no-op. This is fine as we then assume
		// that for this resource (despite operator configuration) the user took the responsibility
		// of configuring RBAC.
		builder.WithServiceAccount(autodiscoverServiceAccountName)
	}

	dataVolume := createDataVolume(params)
	volumes := []volume.VolumeLike{
		volume.NewSecretVolume(
			params.Namer.ConfigSecretName(params.Type, params.Owner.GetName()),
			ConfigVolumeName,
			ConfigMountPath,
			ConfigFileName,
			0600),
		dataVolume,
	}

	if params.Associated.AssociationConf().CAIsConfigured() {
		volumes = append(volumes, volume.NewSecretVolumeWithMountPath(
			params.Associated.AssociationConf().GetCASecretName(),
			CAVolumeName,
			CAMountPath))
	}

	for _, v := range volumes {
		builder = builder.WithVolumes(v.Volume()).WithVolumeMounts(v.VolumeMount())
	}

	builder = builder.WithLabels(commonhash.SetTemplateHashLabel(params.Labels, builder.PodTemplate))

	return builder.PodTemplate
}

func createDataVolume(dp DriverParams) volume.VolumeLike {
	dataMountPath := fmt.Sprintf(DataPathTemplate, dp.Type)
	hostDataPath := fmt.Sprintf(DataMountPathTemplate, dp.Owner.GetNamespace(), dp.Owner.GetName(), dp.Type)

	return volume.NewHostVolume(
		DataVolumeName,
		hostDataPath,
		dataMountPath,
		false,
		corev1.HostPathDirectoryOrCreate)
}
