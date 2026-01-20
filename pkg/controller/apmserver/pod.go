// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"fmt"
	"path/filepath"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	// HTTPPort is the (default) port used by ApmServer
	HTTPPort = DefaultHTTPPort

	SecretTokenKey string = "secret-token"

	DataVolumePath   = ApmBaseDir + "/data"
	ConfigVolumePath = ApmBaseDir + "/config"
)

var (
	DefaultMemoryLimits = resource.MustParse("512Mi")
	DefaultResources    = corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimits,
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimits,
		},
	}
)

// readinessProbe is the readiness probe for the APM Server container
func readinessProbe(tls bool) corev1.Probe {
	scheme := corev1.URISchemeHTTP
	if tls {
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
				Path:   "/",
				Scheme: scheme,
			},
		},
	}
}

var args = []string{
	// -e flag is implicit in containerised versions of APM server as they start the binary with the --environment=container flag.
	"-c", "config/config-secret/apm-server.yml",
}

const (
	dataVolumeName   = "apmserver-data"
	configVolumeName = "config-volume"
)

var (
	configVolume = volume.NewEmptyDirVolume(configVolumeName, ConfigVolumePath)
	// dataVolume is used to propagatee the keystore to the APM server from the keystore init container
	// and to hold metadata written by APM server (at least since 9.0) to avoid writing into the containers filesystem.
	// Given that APM server is stateless we should be OK with an emptyDir volume.
	dataVolume = volume.NewEmptyDirVolume(dataVolumeName, DataVolumePath)
)

type PodSpecParams struct {
	Version         string
	CustomImageName string

	PodTemplate corev1.PodTemplateSpec

	TokenSecret  corev1.Secret
	ConfigSecret corev1.Secret

	keystoreResources *keystore.Resources
}

func newPodSpec(c k8s.Client, as *apmv1.ApmServer, p PodSpecParams, meta metadata.Metadata, setDefaultSecurityContext bool) (corev1.PodTemplateSpec, error) {
	labels := as.GetIdentityLabels()
	labels[APMVersionLabelName] = p.Version

	// ensure the Pod gets rotated on config change
	configHash, err := buildConfigHash(c, as, p)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	annotations := map[string]string{configHashAnnotationName: configHash}

	configSecretVolume := volume.NewSecretVolumeWithMountPath(
		p.ConfigSecret.Name,
		"config",
		filepath.Join(ConfigVolumePath, "config-secret"),
	)

	env := defaults.ExtendPodDownwardEnvVars(corev1.EnvVar{
		Name: "SECRET_TOKEN",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: p.TokenSecret.Name},
				Key:                  SecretTokenKey,
			},
		},
	})

	ports := getDefaultContainerPorts(*as)

	volumes := []corev1.Volume{configVolume.Volume(), configSecretVolume.Volume(), dataVolume.Volume()}
	volumeMounts := []corev1.VolumeMount{configVolume.VolumeMount(), configSecretVolume.VolumeMount(), dataVolume.VolumeMount()}
	var initContainers []corev1.Container

	if p.keystoreResources != nil {
		volumes = append(volumes, p.keystoreResources.Volume)
		initContainers = append(initContainers, p.keystoreResources.InitContainer)
	}

	v, err := version.Parse(p.Version)
	if err != nil {
		return corev1.PodTemplateSpec{}, err // error unlikely and should have been caught during validation
	}

	meta = metadata.Propagate(as, metadata.Metadata{Labels: labels, Annotations: annotations})
	builder := defaults.NewPodTemplateBuilder(p.PodTemplate, apmv1.ApmServerContainerName).
		WithLabels(meta.Labels).
		WithAnnotations(meta.Annotations).
		WithResources(DefaultResources).
		WithDockerImage(p.CustomImageName, container.ImageRepository(container.APMServerImage, v)).
		WithReadinessProbe(readinessProbe(as.Spec.HTTP.TLS.Enabled())).
		WithPorts(ports).
		WithArgs(args...).
		WithEnv(env...).
		WithVolumes(volumes...).
		WithVolumeMounts(volumeMounts...).
		WithInitContainers(initContainers...)

	builder, err = withAssociationCACertsVolumes(builder, *as)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	builder = withHTTPCertsVolume(builder, *as)

	if setDefaultSecurityContext {
		builder = builder.WithPodSecurityContext(corev1.PodSecurityContext{
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		})
	}

	return builder.WithInitContainerDefaults().PodTemplate, nil
}

func getDefaultContainerPorts(as apmv1.ApmServer) []corev1.ContainerPort {
	return []corev1.ContainerPort{{Name: as.Spec.HTTP.Protocol(), ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP}}
}

func withHTTPCertsVolume(builder *defaults.PodTemplateBuilder, as apmv1.ApmServer) *defaults.PodTemplateBuilder {
	if !as.Spec.HTTP.TLS.Enabled() {
		return builder
	}
	vol := certificates.HTTPCertSecretVolume(Namer, as.Name)
	return builder.WithVolumes(vol.Volume()).WithVolumeMounts(vol.VolumeMount())
}

func withAssociationCACertsVolumes(builder *defaults.PodTemplateBuilder, as apmv1.ApmServer) (*defaults.PodTemplateBuilder, error) {
	for _, association := range as.GetAssociations() {
		assocConf, err := association.AssociationConf()
		if err != nil {
			return nil, err
		}
		if !assocConf.CAIsConfigured() {
			continue
		}

		vol := volume.NewSecretVolumeWithMountPath(
			assocConf.GetCASecretName(),
			fmt.Sprintf("%s-certs", association.AssociationType()),
			filepath.Join(ApmBaseDir, certificatesDir(association.AssociationType())),
		)

		builder.WithVolumes(vol.Volume()).WithVolumeMounts(vol.VolumeMount())
	}
	return builder, nil
}
