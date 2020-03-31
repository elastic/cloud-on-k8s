// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"fmt"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
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
		{Name: "ENT_SEARCH_CONFIG_PATH", Value: filepath.Join(ConfigMountPath, ConfigFilename)},
	}
)

func readinessProbe(ents entsv1beta1.EnterpriseSearch) corev1.Probe {
	return corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 60, // initial startup is pretty slow
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      5,
		Handler: corev1.Handler{
			Exec: &corev1.ExecAction{
				Command: []string{"bash", "-c",
					fmt.Sprintf(
						`curl -o /dev/null -w "%%{http_code}" %s://127.0.0.1:%d/swiftype-app-version -k -s`,
						ents.Spec.HTTP.Protocol(),
						HTTPPort,
					),
				},
			},
		},
	}
}

func newPodSpec(ents entsv1beta1.EnterpriseSearch, configHash string) corev1.PodTemplateSpec {
	cfgVolume := ConfigSecretVolume(ents)

	builder := defaults.NewPodTemplateBuilder(
		ents.Spec.PodTemplate, entsv1beta1.EnterpriseSearchContainerName).
		WithResources(DefaultResources).
		WithDockerImage(ents.Spec.Image, container.ImageRepository(container.EnterpriseSearchImage, ents.Spec.Version)).
		WithPorts([]corev1.ContainerPort{
			{Name: ents.Spec.HTTP.Protocol(), ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
		}).
		WithReadinessProbe(readinessProbe(ents)).
		WithVolumes(cfgVolume.Volume()).
		WithVolumeMounts(cfgVolume.VolumeMount()).
		WithEnv(DefaultEnv...).
		// ensure the Pod gets rotated on config change
		WithLabels(map[string]string{ConfigHashLabelName: configHash})

	builder = withESCertsVolume(builder, ents)
	builder = withHTTPCertsVolume(builder, ents)

	return builder.PodTemplate
}

func withESCertsVolume(builder *defaults.PodTemplateBuilder, ents entsv1beta1.EnterpriseSearch) *defaults.PodTemplateBuilder {
	if !ents.AssociationConf().CAIsConfigured() {
		return builder
	}
	vol := volume.NewSecretVolumeWithMountPath(
		ents.AssociationConf().GetCASecretName(),
		"es-certs",
		ESCertsPath,
	)
	return builder.
		WithVolumes(vol.Volume()).
		WithVolumeMounts(vol.VolumeMount())
}

func withHTTPCertsVolume(builder *defaults.PodTemplateBuilder, ents entsv1beta1.EnterpriseSearch) *defaults.PodTemplateBuilder {
	if !ents.Spec.HTTP.TLS.Enabled() {
		return builder
	}
	vol := certificates.HTTPCertSecretVolume(name.EntSearchNamer, ents.Name)
	return builder.WithVolumes(vol.Volume()).WithVolumeMounts(vol.VolumeMount())
}
