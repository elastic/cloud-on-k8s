// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"fmt"
	"hash"

	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	commonhash "github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
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

	// ConfigChecksumLabel is a label used to store a Beat config checksum.
	ConfigChecksumLabel = "beat.k8s.elastic.co/config-checksum"

	// VersionLabelName is a label used to track the version of a Beat Pod.
	VersionLabelName = "beat.k8s.elastic.co/version"
)

var (
	defaultResources = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("200Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("200Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
	}
)

func buildPodTemplate(params DriverParams, defaultImage container.Image, modifyPodFunc func(builder *defaults.PodTemplateBuilder), configHash hash.Hash) corev1.PodTemplateSpec {
	podTemplate := params.GetPodTemplate()

	// Token mounting gets defaulted to false, which prevents from detecting whether user had set it.
	// Instead, checking that here, before the default is applied.
	// This is required for autodiscover which is enabled by default.
	if podTemplate.Spec.AutomountServiceAccountToken == nil {
		t := true
		podTemplate.Spec.AutomountServiceAccountToken = &t
	}

	spec := params.Beat.Spec
	builder := defaults.NewPodTemplateBuilder(podTemplate, spec.Type)

	// might be nil if caller wants to use the default builder without any modifications
	if modifyPodFunc != nil {
		modifyPodFunc(builder)
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
			ConfigChecksumLabel: fmt.Sprintf("%x", configHash.Sum(nil)),
			VersionLabelName:    spec.Version}).
		WithDockerImage(spec.Image, container.ImageRepository(defaultImage, spec.Version)).
		WithArgs("-e", "-c", ConfigMountPath).
		WithDNSPolicy(corev1.DNSClusterFirstWithHostNet).
		WithSecurityContext(corev1.SecurityContext{
			RunAsUser: pointer.Int64(0),
		})

	if ShouldSetupAutodiscoverRBAC() {
		autodiscoverServiceAccountName := ServiceAccountName(params.Beat.Name)
		// If SA is already provided, the call will be no-op. This is fine as we then assume
		// that for this resource (despite operator configuration) the user took the responsibility
		// of configuring RBAC.
		builder.WithServiceAccount(autodiscoverServiceAccountName)
	}

	dataVolume := createDataVolume(params)
	volumes := []volume.VolumeLike{
		volume.NewSecretVolume(
			ConfigSecretName(spec.Type, params.Beat.Name),
			ConfigVolumeName,
			ConfigMountPath,
			ConfigFileName,
			0600),
		dataVolume,
	}

	if params.Beat.AssociationConf().CAIsConfigured() {
		volumes = append(volumes, volume.NewSecretVolumeWithMountPath(
			params.Beat.AssociationConf().GetCASecretName(),
			CAVolumeName,
			CAMountPath))
	}

	for _, v := range volumes {
		builder = builder.WithVolumes(v.Volume()).WithVolumeMounts(v.VolumeMount())
	}

	builder = builder.WithLabels(commonhash.SetTemplateHashLabel(NewLabels(params.Beat), builder.PodTemplate))

	return builder.PodTemplate
}

func createDataVolume(dp DriverParams) volume.VolumeLike {
	dataMountPath := fmt.Sprintf(DataPathTemplate, dp.Beat.Spec.Type)
	hostDataPath := fmt.Sprintf(DataMountPathTemplate, dp.Beat.Namespace, dp.Beat.Name, dp.Beat.Spec.Type)

	return volume.NewHostVolume(
		DataVolumeName,
		hostDataPath,
		dataMountPath,
		false,
		corev1.HostPathDirectoryOrCreate)
}
