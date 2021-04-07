// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package maps

import (
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch/name"
	corev1 "k8s.io/api/core/v1"
)

const (
	HTTPPort           = 8080
	configHashLabel    = "maps.k8s.elastic.co/config-hash"
	logVolumeMountPath = "/var/log/elastic-maps-server"
)

func newPodSpec(ems emsv1alpha1.MapsServer, configHash string) corev1.PodTemplateSpec {
	// ensure the Pod gets rotated on config change
	labels := map[string]string{configHashLabel: configHash}

	defaultContainerPorts := []corev1.ContainerPort{
		{Name: ems.Spec.HTTP.Protocol(), ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
	}

	cfgVolume := configSecretVolume(ems)
	logsVolume := volume.NewEmptyDirVolume("logs", logVolumeMountPath)

	builder := defaults.NewPodTemplateBuilder(ems.Spec.PodTemplate, emsv1alpha1.MapsContainerName).
		WithLabels(labels).
		// TODO WithResources(DefaultResources).
		WithDockerImage(ems.Spec.Image, container.ImageRepository(container.MapsImage, ems.Spec.Version)).
		WithPorts(defaultContainerPorts).
		WithVolumes(cfgVolume.Volume(), logsVolume.Volume()).
		WithVolumeMounts(cfgVolume.VolumeMount(), logsVolume.VolumeMount()).
		WithInitContainerDefaults()

	builder = withESCertsVolume(builder, ems)
	builder = withHTTPCertsVolume(builder, ems)

	return builder.PodTemplate
}

func withESCertsVolume(builder *defaults.PodTemplateBuilder, ems emsv1alpha1.MapsServer) *defaults.PodTemplateBuilder {
	if !ems.AssociationConf().CAIsConfigured() {
		return builder
	}
	vol := volume.NewSecretVolumeWithMountPath(
		ems.AssociationConf().GetCASecretName(),
		"es-certs",
		ESCertsPath,
	)
	return builder.
		WithVolumes(vol.Volume()).
		WithVolumeMounts(vol.VolumeMount())
}

func withHTTPCertsVolume(builder *defaults.PodTemplateBuilder, ems emsv1alpha1.MapsServer) *defaults.PodTemplateBuilder {
	if !ems.Spec.HTTP.TLS.Enabled() {
		return builder
	}
	vol := certificates.HTTPCertSecretVolume(name.EntNamer, ems.Name)
	return builder.WithVolumes(vol.Volume()).WithVolumeMounts(vol.VolumeMount())
}
