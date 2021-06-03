// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"strings"

	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	esvolume "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

var (
	EsLogStyleEnvVarKey   = "ES_LOG_STYLE"
	EsLogStyleEnvVarValue = "file"
)

func IsMonitoringDefined(es esv1.Elasticsearch) bool {
	return IsMonitoringMetricsDefined(es) || IsMonitoringLogDefined(es)
}

func IsMonitoringMetricsDefined(es esv1.Elasticsearch) bool {
	return es.Spec.Monitoring.Metrics.ElasticsearchRef.IsDefined()
}

func IsMonitoringLogDefined(es esv1.Elasticsearch) bool {
	return es.Spec.Monitoring.Logs.ElasticsearchRef.IsDefined()
}

func EnableStackLoggingEnvVar(builder *defaults.PodTemplateBuilder) *defaults.PodTemplateBuilder {
	return builder.WithEnv(corev1.EnvVar{Name: EsLogStyleEnvVarKey, Value: EsLogStyleEnvVarValue})
}

// WithMonitoring updates the Elasticsearch Pod template builder to deploy Metricbeat and Filebeat in sidecar containers
// in the Elasticsearch pod and injects volumes for Metricbeat/Filebeat configs and ES source/target CA certs.
func WithMonitoring(builder *defaults.PodTemplateBuilder, es esv1.Elasticsearch) (*defaults.PodTemplateBuilder, error) {
	// Inject volumes
	builder = builder.WithVolumes(monitoringVolumes(es)...)

	if IsMonitoringMetricsDefined(es) {
		// Inject Metricbeat sidecar container
		metricBeat, err := metricbeatContainer(es)
		if err != nil {
			return nil, err
		}
		builder.PodTemplate.Spec.Containers = append(builder.PodTemplate.Spec.Containers, metricBeat)
	}

	if IsMonitoringLogDefined(es) {
		// Inject Filebeat sidecar container
		filebeat, err := filebeatContainer(es)
		if err != nil {
			return nil, err
		}
		builder.PodTemplate.Spec.Containers = append(builder.PodTemplate.Spec.Containers, filebeat)
	}

	return builder, nil
}

func metricbeatContainer(es esv1.Elasticsearch) (corev1.Container, error) {
	image, err := containerImage(es, container.MetricbeatImage)
	if err != nil {
		return corev1.Container{}, err
	}

	assocConf := es.GetMonitoringMetricsAssociation().AssociationConf()
	envVars := append(monitoringSourceEnvVars(es), monitoringTargetEnvVars(assocConf)...)

	return corev1.Container{
		Name:  MetricbeatContainerName,
		Image: image,
		Args:  []string{"-c", MetricbeatConfigMountPath, "-e"},
		Env:   append(envVars, defaults.PodDownwardEnvVars()...),
		VolumeMounts: []corev1.VolumeMount{
			metricbeatConfigMapVolume(es).VolumeMount(),
			monitoringMetricsSourceCaCertSecretVolume(es).VolumeMount(),
			monitoringMetricsTargetCaCertSecretVolume(es).VolumeMount(),
		},
	}, nil
}

func filebeatContainer(es esv1.Elasticsearch) (corev1.Container, error) {
	image, err := containerImage(es, container.FilebeatImage)
	if err != nil {
		return corev1.Container{}, err
	}

	assocConf := es.GetMonitoringLogsAssociation().AssociationConf()
	envVars := monitoringTargetEnvVars(assocConf)

	return corev1.Container{
		Name:  FilebeatContainerName,
		Image: image,
		Args:  []string{"-c", FilebeatConfigMountPath, "-e"},
		Env:   append(envVars, defaults.PodDownwardEnvVars()...),
		VolumeMounts: []corev1.VolumeMount{
			esvolume.DefaultLogsVolumeMount,
			filebeatConfigMapVolume(es).VolumeMount(),
			monitoringLogsTargetCaCertSecretVolume(es).VolumeMount(),
		},
	}, nil
}

// containerImage returns the full Beat container image with the image registry.
// If the Elasticsearch specification is configured with a custom image, we do best effort by trying to derive the Beat
// image from the Elasticsearch custom image with an image name replacement
// (<registry>/elasticsearch/elasticsearch:<version> becomes <registry>/beats/<filebeat|metricbeat>:<version>)
func containerImage(es esv1.Elasticsearch, defaultImage container.Image) (string, error) {
	fullCustomImage := es.Spec.Image
	if fullCustomImage != "" {
		esImage := string(container.ElasticsearchImage)
		// Check if Elasticsearch image follows official Elastic naming
		if strings.Contains(fullCustomImage, esImage) {
			// Derive the Beat image from the ES custom image, there is no guarantee that the resulted image exists
			return strings.ReplaceAll(fullCustomImage, esImage, string(defaultImage)), nil
		}
		return "", errors.New("stack monitoring not supported with custom image")
	}
	return container.ImageRepository(defaultImage, es.Spec.Version), nil
}

func monitoringSourceEnvVars(es esv1.Elasticsearch) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: EsSourceURLEnvVarKey, Value: EsSourceURLEnvVarValue},
		{Name: EsSourceUsernameEnvVarKey, Value: user.ElasticUserName},
		{Name: EsSourcePasswordEnvVarKey, ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: esv1.ElasticUserSecret(es.Name)},
				Key: user.ElasticUserName,
			},
		}},
	}
}

func monitoringTargetEnvVars(assocConf *v1.AssociationConf) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: EsTargetURLEnvVarKey, Value: assocConf.GetURL()},
		{Name: EsTargetUsernameEnvVarKey, Value: assocConf.GetAuthSecretKey()},
		{Name: EsTargetPasswordEnvVarKey, ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: assocConf.GetAuthSecretName(),
				},
				Key: assocConf.GetAuthSecretKey(),
			},
		}},
	}
}
