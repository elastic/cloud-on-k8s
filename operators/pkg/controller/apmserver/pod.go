// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/apmserver/config"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
)

const (
	// HTTPPort is the (default) port used by ApmServer
	HTTPPort = config.DefaultHTTPPort

	defaultImageRepositoryAndName string = "docker.elastic.co/apm/apm-server"

	SecretTokenKey string = "secret-token"
)

type PodSpecParams struct {
	Version         string
	CustomImageName string

	ApmServerSecret corev1.Secret
	ConfigSecret    corev1.Secret
}

func imageWithVersion(image string, version string) string {
	return stringsutil.Concat(image, ":", version)
}

func NewPodSpec(p PodSpecParams) corev1.PodSpec {
	imageName := p.CustomImageName
	if p.CustomImageName == "" {
		imageName = imageWithVersion(defaultImageRepositoryAndName, p.Version)
	}

	probe := &corev1.Probe{
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

	configVolume := volume.NewEmptyDirVolume("config-volume", "/usr/share/apm-server/config")

	configSecretVolume := volume.NewSecretVolumeWithMountPath(
		p.ConfigSecret.Name,
		"config",
		"/usr/share/apm-server/config/config-secret",
	)

	automountServiceAccountToken := false
	return corev1.PodSpec{
		Containers: []corev1.Container{{
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
			},
			Env: []corev1.EnvVar{
				{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
				}},
				{Name: "SECRET_TOKEN", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: p.ApmServerSecret.Name},
						Key:                  SecretTokenKey,
					},
				}},
			},
			Image: imageName,
			Name:  "apm-server",
			Command: []string{
				"apm-server",
				"run",
				"-e", // log to stderr
				"-c", "config/config-secret/apm-server.yml",
			},
			Ports: []corev1.ContainerPort{
				{Name: "http", ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
			},
			ReadinessProbe: probe,
			VolumeMounts: []corev1.VolumeMount{
				configVolume.VolumeMount(),
				configSecretVolume.VolumeMount(),
			},
		}},
		AutomountServiceAccountToken: &automountServiceAccountToken,
		Volumes: []corev1.Volume{
			configVolume.Volume(),
			configSecretVolume.Volume(),
		},
	}
}
