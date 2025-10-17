// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package packageregistry

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/epr/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
)

const (
	HTTPPort                 = 8080
	configHashAnnotationName = "packageregistry.k8s.elastic.co/config-hash"
	TLSKeyEnvName            = "EPR_TLS_KEY"
	TLSCertEnvName           = "EPR_TLS_CERT"
	AddressEnvName           = "EPR_ADDRESS"
)

var (
	DefaultMemoryReqs   = resource.MustParse("1Gi")
	DefaultCPUReqs      = resource.MustParse("500m")
	DefaultMemoryLimits = resource.MustParse("4Gi")
	DefaultCPULimits    = resource.MustParse("1000m")
	DefaultResources    = corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryReqs,
			corev1.ResourceCPU:    DefaultCPUReqs,
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimits,
			corev1.ResourceCPU:    DefaultCPULimits,
		},
	}
)

// readinessProbe is the readiness probe for the epr container
func readinessProbe(useTLS bool) corev1.Probe {
	scheme := corev1.URISchemeHTTP
	if useTLS {
		scheme = corev1.URISchemeHTTPS
	}
	return corev1.Probe{
		FailureThreshold:    16,
		InitialDelaySeconds: 120,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      30,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(HTTPPort),
				Path:   "/health",
				Scheme: scheme,
			},
		},
	}
}

func newPodSpec(epr eprv1alpha1.ElasticPackageRegistry, configHash string, meta metadata.Metadata) (corev1.PodTemplateSpec, error) {
	// ensure the Pod gets rotated on config change
	podMeta := meta.Merge(metadata.Metadata{Annotations: map[string]string{configHashAnnotationName: configHash}})

	defaultContainerPorts := []corev1.ContainerPort{
		{Name: epr.Spec.HTTP.Protocol(), ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
	}

	builder := defaults.NewPodTemplateBuilder(epr.Spec.PodTemplate, eprv1alpha1.EPRContainerName)

	v, err := version.Parse(epr.Spec.Version)
	if err != nil {
		return corev1.PodTemplateSpec{}, err // error unlikely and should have been caught during validation
	}

	eprVars := []corev1.EnvVar{
		{Name: AddressEnvName, Value: "0.0.0.0:8080"},
	}

	if epr.Spec.HTTP.TLS.Enabled() {
		eprVars = append(eprVars, corev1.EnvVar{Name: TLSKeyEnvName, Value: "/mnt/elastic-internal/http-certs/tls.key"})
		eprVars = append(eprVars, corev1.EnvVar{Name: TLSCertEnvName, Value: "/mnt/elastic-internal/http-certs/tls.crt"})
	}

	builder = builder.
		WithAnnotations(podMeta.Annotations).
		WithLabels(podMeta.Labels).
		WithResources(DefaultResources).
		WithDockerImage(epr.Spec.Image, container.ImageRepository(container.PackageRegistryImage, v)).
		WithReadinessProbe(readinessProbe(epr.Spec.HTTP.TLS.Enabled())).
		WithPorts(defaultContainerPorts).
		WithInitContainerDefaults().
		WithEnv(eprVars...).
		WithContainersSecurityContext(corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			Privileged:             ptr.To(false),
			ReadOnlyRootFilesystem: ptr.To(true),
			RunAsUser:              ptr.To(int64(1000)),
			RunAsGroup:             ptr.To(int64(1000)),
		})

	// Add configuration volume
	configVolume := configSecretVolume(epr)
	builder = builder.WithVolumes(configVolume.Volume()).WithVolumeMounts(configVolume.VolumeMount())

	// Add HTTP certificates volume if TLS is enabled
	builder = withHTTPCertsVolume(builder, epr)

	return builder.PodTemplate, nil
}

func withHTTPCertsVolume(builder *defaults.PodTemplateBuilder, epr eprv1alpha1.ElasticPackageRegistry) *defaults.PodTemplateBuilder {
	if !epr.Spec.HTTP.TLS.Enabled() {
		return builder
	}
	vol := certificates.HTTPCertSecretVolume(eprv1alpha1.Namer, epr.Name)
	return builder.WithVolumes(vol.Volume()).WithVolumeMounts(vol.VolumeMount())
}
