// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package enterprisesearch

import (
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	entv1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
)

const (
	EnvJavaOpts              = "JAVA_OPTS"
	HTTPPort                 = 3002
	DefaultJavaOpts          = "-Xms3500m -Xmx3500m"
	ConfigHashAnnotationName = "enterprisesearch.k8s.elastic.co/config-hash"
	LogVolumeMountPath       = "/var/log/enterprise-search"
)

var (
	DefaultMemoryLimits = resource.MustParse("4Gi")
	DefaultResources    = corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimits,
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimits,
		},
	}
	DefaultEnv = []corev1.EnvVar{
		{Name: EnvJavaOpts, Value: DefaultJavaOpts},
	}
	ReadinessProbe = corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 60, // initial startup is pretty slow
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      5,
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"bash", path.Join(ReadinessProbeMountPath)},
			},
		},
	}
)

func newPodSpec(ent entv1.EnterpriseSearch, configHash string) (corev1.PodTemplateSpec, error) {
	// ensure the Pod gets rotated on config change
	annotations := map[string]string{ConfigHashAnnotationName: configHash}

	defaultContainerPorts := []corev1.ContainerPort{
		{Name: ent.Spec.HTTP.Protocol(), ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
	}

	cfgVolume := ConfigSecretVolume(ent)
	readinessProbeVolume := ReadinessProbeSecretVolume(ent)
	logsVolume := volume.NewEmptyDirVolume("logs", LogVolumeMountPath)

	builder := defaults.NewPodTemplateBuilder(ent.Spec.PodTemplate, entv1.EnterpriseSearchContainerName).
		WithAnnotations(annotations).
		WithResources(DefaultResources).
		WithDockerImage(ent.Spec.Image, container.ImageRepository(container.EnterpriseSearchImage, ent.Spec.Version)).
		WithPorts(defaultContainerPorts).
		WithReadinessProbe(ReadinessProbe).
		WithEnv(DefaultEnv...).
		WithVolumes(cfgVolume.Volume(), readinessProbeVolume.Volume(), logsVolume.Volume()).
		WithVolumeMounts(cfgVolume.VolumeMount(), readinessProbeVolume.VolumeMount(), logsVolume.VolumeMount()).
		WithInitContainerDefaults()

	builder, err := withESCertsVolume(builder, ent)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	builder = withHTTPCertsVolume(builder, ent)

	return builder.PodTemplate, nil
}

func withESCertsVolume(builder *defaults.PodTemplateBuilder, ent entv1.EnterpriseSearch) (*defaults.PodTemplateBuilder, error) {
	esAssocConf, err := ent.AssociationConf()
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

func withHTTPCertsVolume(builder *defaults.PodTemplateBuilder, ent entv1.EnterpriseSearch) *defaults.PodTemplateBuilder {
	if !ent.Spec.HTTP.TLS.Enabled() {
		return builder
	}
	vol := certificates.HTTPCertSecretVolume(entv1.Namer, ent.Name)
	return builder.WithVolumes(vol.Volume()).WithVolumeMounts(vol.VolumeMount())
}
