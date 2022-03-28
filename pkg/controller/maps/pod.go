// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package maps

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	emsv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
)

const (
	HTTPPort                 = 8080
	configHashAnnotationName = "maps.k8s.elastic.co/config-hash"
	logVolumeMountPath       = "/var/log/elastic-maps-server"
)

var (
	DefaultMemoryLimits = resource.MustParse("200Mi")
	DefaultResources    = corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimits,
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimits,
		},
	}
)

// readinessProbe is the readiness probe for the maps container
func readinessProbe(useTLS bool) corev1.Probe {
	scheme := corev1.URISchemeHTTP
	if useTLS {
		scheme = corev1.URISchemeHTTPS
	}
	return corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 10,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      5,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(HTTPPort),
				Path:   "/status",
				Scheme: scheme,
			},
		},
	}
}

func newPodSpec(ems emsv1alpha1.ElasticMapsServer, configHash string) (corev1.PodTemplateSpec, error) {
	// ensure the Pod gets rotated on config change
	annotations := map[string]string{configHashAnnotationName: configHash}

	defaultContainerPorts := []corev1.ContainerPort{
		{Name: ems.Spec.HTTP.Protocol(), ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
	}

	cfgVolume := configSecretVolume(ems)
	logsVolume := volume.NewEmptyDirVolume("logs", logVolumeMountPath)

	builder := defaults.NewPodTemplateBuilder(ems.Spec.PodTemplate, emsv1alpha1.MapsContainerName).
		WithAnnotations(annotations).
		WithResources(DefaultResources).
		WithDockerImage(ems.Spec.Image, container.ImageRepository(container.MapsImage, ems.Spec.Version)).
		WithReadinessProbe(readinessProbe(ems.Spec.HTTP.TLS.Enabled())).
		WithPorts(defaultContainerPorts).
		WithVolumes(cfgVolume.Volume(), logsVolume.Volume()).
		WithVolumeMounts(cfgVolume.VolumeMount(), logsVolume.VolumeMount()).
		WithInitContainerDefaults()

	builder, err := withESCertsVolume(builder, ems)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	builder = withHTTPCertsVolume(builder, ems)

	esAssocConf, err := ems.AssociationConf()
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	if !esAssocConf.IsConfigured() {
		// supported as of 7.14, harmless on prior versions, but both Elasticsearch connection and this must not be specified
		builder = builder.WithEnv(corev1.EnvVar{Name: "ELASTICSEARCH_PREVALIDATED", Value: "true"})
	}

	return builder.PodTemplate, nil
}

func withESCertsVolume(builder *defaults.PodTemplateBuilder, ems emsv1alpha1.ElasticMapsServer) (*defaults.PodTemplateBuilder, error) {
	esAssocConf, err := ems.AssociationConf()
	if err != nil {
		return nil, err
	}
	if !esAssocConf.CAIsConfigured() {
		return builder, nil
	}
	vol := volume.NewSecretVolumeWithMountPath(
		esAssocConf.GetCASecretName(),
		"es-certs",
		ESCertsPath,
	)
	return builder.
		WithVolumes(vol.Volume()).
		WithVolumeMounts(vol.VolumeMount()), nil
}

func withHTTPCertsVolume(builder *defaults.PodTemplateBuilder, ems emsv1alpha1.ElasticMapsServer) *defaults.PodTemplateBuilder {
	if !ems.Spec.HTTP.TLS.Enabled() {
		return builder
	}
	vol := certificates.HTTPCertSecretVolume(EMSNamer, ems.Name)
	return builder.WithVolumes(vol.Volume()).WithVolumeMounts(vol.VolumeMount())
}
