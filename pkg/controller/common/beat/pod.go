// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"fmt"
	"hash"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	commonhash "github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	corev1 "k8s.io/api/core/v1"
)

func buildPodTemplate(params DriverParams, defaultImage container.Image, f func(builder *defaults.PodTemplateBuilder), checksum hash.Hash) corev1.PodTemplateSpec {
	podTemplate := params.GetPodTemplate()

	// Token mounting gets defaulted to false, which prevents from detecting whether user set it.
	// Instead, checking that here, before the default is applied.
	if podTemplate.Spec.AutomountServiceAccountToken == nil {
		t := true
		podTemplate.Spec.AutomountServiceAccountToken = &t
	}

	builder := defaults.NewPodTemplateBuilder(podTemplate, params.Type).
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
		WithArgs("-e", "-c", ConfigMountPath(params.Type)).
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

	dataVolume, _ := createDataVolume(params) //todo
	volumes := []volume.VolumeLike{
		volume.NewSecretVolume(
			params.Namer.ConfigSecretName(params.Type, params.Owner.GetName()),
			ConfigVolumeName,
			ConfigMountPath(params.Type),
			configFileName(params.Type),
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

	if f != nil {
		f(builder)
	}

	builder = builder.WithLabels(commonhash.SetTemplateHashLabel(params.Labels, builder.PodTemplate))

	return builder.PodTemplate
}

func createDataVolume(dp DriverParams) (volume.VolumeLike, error) {
	var v volume.VolumeLike
	dataMountPath := fmt.Sprintf("/usr/share/%s/data", dp.Type)
	switch {
	case dp.DaemonSet != nil:
		{
			hostDataPath := fmt.Sprintf(
				"/var/lib/%s/%s/%s-data",
				dp.Owner.GetNamespace(),
				dp.Owner.GetName(),
				dp.Type)
			v = volume.NewHostVolume(
				"data",
				hostDataPath,
				dataMountPath,
				false,
				corev1.HostPathDirectoryOrCreate)
		}
	case dp.Deployment != nil:
		{
			v = volume.NewPersistentVolumeClaim(
				"data",
				dataMountPath)
		}
	default:
		return nil, fmt.Errorf("both daemonset and deployment are nil")
	}

	return v, nil
}
