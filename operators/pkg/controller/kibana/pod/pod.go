// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// HTTPPort is the (default) port used by Kibana
	HTTPPort                             = 5601
	defaultImageRepositoryAndName string = "docker.elastic.co/kibana/kibana"
)

// DefaultResources are resource limits to apply to Kibana container by default
var DefaultResources = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
}

func imageWithVersion(image string, version string) string {
	return stringsutil.Concat(image, ":", version)
}

type EnvFactory func(kibana v1alpha1.Kibana) []corev1.EnvVar

func NewPodTemplateSpec(kb v1alpha1.Kibana) corev1.PodTemplateSpec {
	// inherit from the user-provided podTemplateSpec
	objectMeta := kb.Spec.PodTemplate.ObjectMeta.DeepCopy()
	spec := kb.Spec.PodTemplate.Spec.DeepCopy()

	// add (or override) our labels on top of user-provided ones
	if objectMeta.Labels == nil {
		objectMeta.Labels = map[string]string{}
	}
	for k, v := range label.NewLabels(kb.Name) {
		objectMeta.Labels[k] = v
	}

	// disable service account token automount unless enabled by the user
	varFalse := false
	if spec.AutomountServiceAccountToken == nil {
		spec.AutomountServiceAccountToken = &varFalse
	}

	userProvidedContainerSpec := true
	kibanaContainer := GetKibanaContainer(kb.Spec.PodTemplate.Spec).DeepCopy()
	if kibanaContainer == nil {
		userProvidedContainerSpec = false
		kibanaContainer = &corev1.Container{Name: v1alpha1.KibanaContainerName}
	}

	// set Docker image name if not user-provided
	imageName := imageWithVersion(defaultImageRepositoryAndName, kb.Spec.Version)
	if kb.Spec.Image != "" {
		imageName = kb.Spec.Image
	}
	if kibanaContainer.Image != "" {
		imageName = kibanaContainer.Image
	}
	kibanaContainer.Image = imageName

	// set readiness probe
	kibanaContainer.ReadinessProbe = &corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 10,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      5,
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(HTTPPort),
				Path:   "/",
				Scheme: corev1.URISchemeHTTP,
			},
		},
	}

	// set resource requirements if not user-provided
	if len(kibanaContainer.Resources.Limits) == 0 && len(kibanaContainer.Resources.Requests) == 0 {
		kibanaContainer.Resources = DefaultResources
	}

	// set our ports to the Kibana container
	kibanaContainer.Ports = []corev1.ContainerPort{
		{Name: "http", ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
	}

	// set the modified Kibana container back into the spec
	if userProvidedContainerSpec {
		for i, c := range spec.Containers {
			if c.Name == v1alpha1.KibanaContainerName {
				spec.Containers[i] = *kibanaContainer
			}
		}
	} else {
		spec.Containers = append(spec.Containers, *kibanaContainer)
	}

	return corev1.PodTemplateSpec{
		ObjectMeta: *objectMeta,
		Spec:       *spec,
	}
}

// GetKibanaContainer returns the Kibana container from the given podSpec.
// It returns nil if the container does not exist.
// Warning: this function returns a pointer to the object that can then be mutated.
func GetKibanaContainer(podSpec corev1.PodSpec) *corev1.Container {
	for i, c := range podSpec.Containers {
		if c.Name == v1alpha1.KibanaContainerName {
			return &podSpec.Containers[i]
		}
	}
	return nil
}
