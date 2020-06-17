// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch/name"
)

const (
	HTTPPort            = 3002
	DefaultJavaOpts     = "-Xms3500m -Xmx3500m"
	ConfigHashLabelName = "enterprisesearch.k8s.elastic.co/config-hash"
	LogVolumeMountPath  = "/var/log/enterprise-search"
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
		{Name: "JAVA_OPTS", Value: DefaultJavaOpts},
	}
	ReadinessProbe = corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 60, // initial startup is pretty slow
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      5,
		Handler: corev1.Handler{
			Exec: &corev1.ExecAction{
				Command: []string{"bash", path.Join(ReadinessProbeMountPath)},
			},
		},
	}
)

func newPodSpec(ent entv1beta1.EnterpriseSearch, configHash string) corev1.PodTemplateSpec {
	cfgVolume := ConfigSecretVolume(ent)
	readinessProbeVolume := ReadinessProbeSecretVolume(ent)
	logsVolume := volume.NewEmptyDirVolume("logs", LogVolumeMountPath)

	builder := defaults.NewPodTemplateBuilder(
		ent.Spec.PodTemplate, entv1beta1.EnterpriseSearchContainerName).
		WithResources(DefaultResources).
		WithDockerImage(ent.Spec.Image, container.ImageRepository(container.EnterpriseSearchImage, ent.Spec.Version)).
		WithPorts([]corev1.ContainerPort{
			{Name: ent.Spec.HTTP.Protocol(), ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
		}).
		WithReadinessProbe(ReadinessProbe).
		WithVolumes(cfgVolume.Volume(), readinessProbeVolume.Volume(), logsVolume.Volume()).
		WithVolumeMounts(cfgVolume.VolumeMount(), readinessProbeVolume.VolumeMount(), logsVolume.VolumeMount()).
		WithEnv(DefaultEnv...).
		// ensure the Pod gets rotated on config change
		WithLabels(map[string]string{ConfigHashLabelName: configHash})

	builder = withESCertsVolume(builder, ent)
	builder = withHTTPCertsVolume(builder, ent)

	return builder.PodTemplate
}

func withESCertsVolume(builder *defaults.PodTemplateBuilder, ent entv1beta1.EnterpriseSearch) *defaults.PodTemplateBuilder {
	if !ent.AssociationConf().CAIsConfigured() {
		return builder
	}
	vol := volume.NewSecretVolumeWithMountPath(
		ent.AssociationConf().GetCASecretName(),
		"es-certs",
		ESCertsPath,
	)
	return builder.
		WithVolumes(vol.Volume()).
		WithVolumeMounts(vol.VolumeMount())
}

func withHTTPCertsVolume(builder *defaults.PodTemplateBuilder, ent entv1beta1.EnterpriseSearch) *defaults.PodTemplateBuilder {
	if !ent.Spec.HTTP.TLS.Enabled() {
		return builder
	}
	vol := certificates.HTTPCertSecretVolume(name.EntNamer, ent.Name)
	return builder.WithVolumes(vol.Volume()).WithVolumeMounts(vol.VolumeMount())
}
