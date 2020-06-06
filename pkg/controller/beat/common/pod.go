// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"fmt"
	"hash"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
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
	DefaultResources = corev1.ResourceRequirements{
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

func buildPodTemplate(
	params DriverParams,
	defaultImage container.Image,
	podTemplateFunc PodTemplateFunc,
	configHash hash.Hash) corev1.PodTemplateSpec {
	spec := &params.Beat.Spec
	builder := defaults.NewPodTemplateBuilder(params.GetPodTemplate(), spec.Type)

	// might be nil if caller wants to use the default builder without any modifications
	if podTemplateFunc != nil {
		podTemplateFunc(types.NamespacedName{Name: params.Beat.Name, Namespace: params.Beat.Namespace}, builder)
	}

	builder = builder.
		WithLabels(maps.Merge(NewLabels(params.Beat), map[string]string{
			ConfigChecksumLabel: fmt.Sprintf("%x", configHash.Sum(nil)),
			VersionLabelName:    spec.Version})).
		WithDockerImage(spec.Image, container.ImageRepository(defaultImage, spec.Version)).
		WithArgs("-e", "-c", ConfigMountPath)

	if shouldManageRBACFor(builder.PodTemplate.Spec) {
		builder.WithServiceAccount(ServiceAccountName(params.Beat.Name))
		builder.PodTemplate.Spec.AutomountServiceAccountToken = pointer.BoolPtr(true)
	}

	volumes := []volume.VolumeLike{
		volume.NewSecretVolume(
			ConfigSecretName(spec.Type, params.Beat.Name),
			ConfigVolumeName,
			ConfigMountPath,
			ConfigFileName,
			0600),
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

	return builder.PodTemplate
}
