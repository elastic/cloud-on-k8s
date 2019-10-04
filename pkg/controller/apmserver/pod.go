// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"path/filepath"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/config"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// HTTPPort is the (default) port used by ApmServer
	HTTPPort = config.DefaultHTTPPort

	defaultImageRepositoryAndName string = "docker.elastic.co/apm/apm-server"

	SecretTokenKey string = "secret-token"

	DataVolumePath   = ApmBaseDir + "/data"
	ConfigVolumePath = ApmBaseDir + "/config"
)

var DefaultResources = corev1.ResourceRequirements{
	Requests: map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceMemory: resource.MustParse("512Mi"),
	},
	Limits: map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceMemory: resource.MustParse("512Mi"),
	},
}

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
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(HTTPPort),
				Path:   "/",
				Scheme: scheme,
			},
		},
	}
}

var ports = []corev1.ContainerPort{
	{Name: "http", ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
}

var command = []string{
	"apm-server",
	"run",
	"-e", // log to stderr
	"-c", "config/config-secret/apm-server.yml",
}

var configVolume = volume.NewEmptyDirVolume("config-volume", ConfigVolumePath)

type PodSpecParams struct {
	Version         string
	CustomImageName string

	PodTemplate corev1.PodTemplateSpec

	ApmServerSecret corev1.Secret
	ConfigSecret    corev1.Secret

	keystoreResources *keystore.Resources
}

func imageWithVersion(image string, version string) string {
	return stringsutil.Concat(image, ":", version)
}

func newPodSpec(as *v1beta1.ApmServer, p PodSpecParams) corev1.PodTemplateSpec {
	configSecretVolume := volume.NewSecretVolumeWithMountPath(
		p.ConfigSecret.Name,
		"config",
		filepath.Join(ConfigVolumePath, "config-secret"),
	)

	env := append(defaults.PodDownwardEnvVars, corev1.EnvVar{
		Name: "SECRET_TOKEN",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: p.ApmServerSecret.Name},
				Key:                  SecretTokenKey,
			},
		},
	})

	builder := defaults.NewPodTemplateBuilder(
		p.PodTemplate, v1beta1.APMServerContainerName).
		WithResources(DefaultResources).
		WithDockerImage(p.CustomImageName, imageWithVersion(defaultImageRepositoryAndName, p.Version)).
		WithReadinessProbe(readinessProbe(as.Spec.HTTP.TLS.Enabled())).
		WithPorts(ports).
		WithCommand(command).
		WithVolumes(configVolume.Volume(), configSecretVolume.Volume()).
		WithVolumeMounts(configVolume.VolumeMount(), configSecretVolume.VolumeMount()).
		WithEnv(env...)

	if p.keystoreResources != nil {
		dataVolume := keystore.DataVolume(
			strings.ToLower(as.Kind),
			DataVolumePath,
		)
		builder.WithInitContainers(p.keystoreResources.InitContainer).
			WithVolumes(p.keystoreResources.Volume, dataVolume.Volume()).
			WithVolumeMounts(dataVolume.VolumeMount()).
			WithInitContainerDefaults()
	}

	return builder.PodTemplate
}
