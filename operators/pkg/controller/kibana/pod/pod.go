// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// HTTPPort is the (default) port used by Kibana
	HTTPPort                             = 5601
	defaultImageRepositoryAndName string = "docker.elastic.co/kibana/kibana"
)

// ports to set in the Kibana container
var ports = []corev1.ContainerPort{
	{Name: "http", ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
}

// readinessProbe is the readiness probe for the Kibana container
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
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(HTTPPort),
				Path:   "/",
				Scheme: scheme,
			},
		},
	}
}

func imageWithVersion(image string, version string) string {
	return stringsutil.Concat(image, ":", version)
}

func NewPodTemplateSpec(kb v1alpha1.Kibana, keystore *keystore.Resources) corev1.PodTemplateSpec {
	builder := defaults.NewPodTemplateBuilder(kb.Spec.PodTemplate, v1alpha1.KibanaContainerName).
		WithLabels(label.NewLabels(kb.Name)).
		WithDockerImage(kb.Spec.Image, imageWithVersion(defaultImageRepositoryAndName, kb.Spec.Version)).
		WithReadinessProbe(readinessProbe(kb.Spec.HTTP.TLS.Enabled())).
		WithPorts(ports).
		WithVolumes(volume.KibanaDataVolume.Volume()).
		WithVolumeMounts(volume.KibanaDataVolume.VolumeMount())

	if keystore != nil {
		builder.WithVolumes(keystore.Volume).
			WithInitContainers(keystore.InitContainer).
			WithInitContainerDefaults()
	}

	return builder.PodTemplate
}

// GetKibanaContainer returns the Kibana container from the given podSpec.
func GetKibanaContainer(podSpec corev1.PodSpec) *corev1.Container {
	return pod.ContainerByName(podSpec, v1alpha1.KibanaContainerName)
}
