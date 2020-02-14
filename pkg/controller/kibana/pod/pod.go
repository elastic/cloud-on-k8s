// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"k8s.io/apimachinery/pkg/api/resource"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/pod"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/volume"

	corev1 "k8s.io/api/core/v1"
)

const (
	// HTTPPort is the (default) port used by Kibana
	HTTPPort = 5601
)

var (
	DefaultMemoryLimits = resource.MustParse("1Gi")
	DefaultResources    = corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimits,
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimits,
		},
	}

	// DefaultAnnotations are the default annotations for the Kibana pods
	DefaultAnnotations = map[string]string{
		annotation.FilebeatModuleAnnotation: "kibana",
	}
)

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
			Exec: &corev1.ExecAction{
				Command: []string{"bash", "-c",
					fmt.Sprintf(`curl -o /dev/null -w "%%{http_code}" %s://127.0.0.1:%d/login -k -s`, scheme, HTTPPort),
				},
			},
		},
	}
}

func NewPodTemplateSpec(kb kbv1.Kibana, keystore *keystore.Resources) corev1.PodTemplateSpec {
	labels := label.NewLabels(kb.Name)
	labels[label.KibanaVersionLabelName] = kb.Spec.Version
	ports := getDefaultContainerPorts(kb)
	builder := defaults.NewPodTemplateBuilder(kb.Spec.PodTemplate, kbv1.KibanaContainerName).
		WithResources(DefaultResources).
		WithLabels(labels).
		WithAnnotations(DefaultAnnotations).
		WithDockerImage(kb.Spec.Image, container.ImageRepository(container.KibanaImage, kb.Spec.Version)).
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
	return pod.ContainerByName(podSpec, kbv1.KibanaContainerName)
}

func getDefaultContainerPorts(kb kbv1.Kibana) []corev1.ContainerPort {
	return []corev1.ContainerPort{{Name: kb.Spec.HTTP.Protocol(), ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP}}
}
