// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"encoding/base64"
	"fmt"
	"hash"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	commonassociation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/network"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/stackmon"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

const (
	defaultFsGroup = 1000

	// ConfigHashAnnotationName is an annotation used to store the Logstash config hash.
	ConfigHashAnnotationName = "logstash.k8s.elastic.co/config-hash"

	// VersionLabelName is a label used to track the version of a Logstash Pod.
	VersionLabelName = "logstash.k8s.elastic.co/version"

	// EnvJavaOpts is the documented environment variable to set JVM options for Logstash.
	EnvJavaOpts = "LS_JAVA_OPTS"
)

var (
	DefaultMemoryLimit = resource.MustParse("2Gi")
	DefaultResources   = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimit,
			corev1.ResourceCPU:    resource.MustParse("2000m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimit,
			corev1.ResourceCPU:    resource.MustParse("1000m"),
		},
	}

	DefaultSecurityContext = corev1.PodSecurityContext{
		FSGroup: ptr.To[int64](defaultFsGroup),
	}
)

func buildPodTemplate(params Params, configHash hash.Hash32) (corev1.PodTemplateSpec, error) {
	defer tracing.Span(&params.Context)()
	spec := &params.Logstash.Spec
	builder := defaults.NewPodTemplateBuilder(params.GetPodTemplate(), logstashv1alpha1.LogstashContainerName)

	volumes, volumeMounts, err := volume.BuildVolumes(params.Logstash, params.APIServerConfig.UseTLS())
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	esAssociations := getEsAssociations(params)
	if err := writeEsAssocToConfigHash(params, esAssociations, configHash); err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	envs, err := buildEnv(params, esAssociations)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	if err := writeHTTPSCertsToConfigHash(params, configHash); err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	labels := maps.Merge(params.Logstash.GetPodIdentityLabels(), map[string]string{
		VersionLabelName: spec.Version})

	annotations := map[string]string{
		ConfigHashAnnotationName: fmt.Sprint(configHash.Sum32()),
	}

	ports := getDefaultContainerPorts()

	if params.KeystoreResources != nil {
		builder = builder.
			WithVolumes(params.KeystoreResources.Volume).
			WithInitContainers(params.KeystoreResources.InitContainer)
	}

	v, err := version.Parse(spec.Version)
	if err != nil {
		return corev1.PodTemplateSpec{}, err // error unlikely and should have been caught during validation
	}

	builder = builder.
		WithResources(DefaultResources).
		WithLabels(labels).
		WithAnnotations(annotations).
		WithDockerImage(spec.Image, container.ImageRepository(container.LogstashImage, v)).
		WithAutomountServiceAccountToken().
		WithPorts(ports).
		WithReadinessProbe(readinessProbe(params)).
		WithEnv(envs...).
		WithVolumes(volumes...).
		WithVolumeMounts(volumeMounts...).
		WithInitContainers(initConfigContainer(params)).
		WithInitContainerDefaults().
		WithPodSecurityContext(DefaultSecurityContext)

	builder, err = stackmon.WithMonitoring(params.Context, params.Client, builder, params.Logstash, params.APIServerConfig)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	return builder.PodTemplate, nil
}

func getDefaultContainerPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{Name: "http", ContainerPort: int32(network.HTTPPort), Protocol: corev1.ProtocolTCP},
	}
}

// readinessProbe is the readiness probe for the Logstash container
func readinessProbe(params Params) corev1.Probe {
	logstash := params.Logstash

	var scheme = corev1.URISchemeHTTP
	if params.APIServerConfig.UseTLS() {
		scheme = corev1.URISchemeHTTPS
	}

	var port = network.HTTPPort
	for _, service := range logstash.Spec.Services {
		if service.Name == LogstashAPIServiceName && len(service.Service.Spec.Ports) > 0 {
			port = int(service.Service.Spec.Ports[0].Port)
		}
	}

	probe := corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      5,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:        intstr.FromInt(port),
				Path:        "/",
				Scheme:      scheme,
				HTTPHeaders: getHTTPHeaders(params),
			},
		},
	}
	return probe
}

// getHTTPHeaders when api.auth.type is set, take api.auth.basic.username and api.auth.basic.password from logstash.yml
// to build Authorization header
func getHTTPHeaders(params Params) []corev1.HTTPHeader {
	if strings.ToLower(params.APIServerConfig.AuthType) != "basic" {
		return nil
	}

	usernamePassword := fmt.Sprintf("%s:%s", params.APIServerConfig.Username, params.APIServerConfig.Password)
	encodedUsernamePassword := base64.StdEncoding.EncodeToString([]byte(usernamePassword))
	authHeader := corev1.HTTPHeader{Name: "Authorization", Value: fmt.Sprintf("Basic %s", encodedUsernamePassword)}

	return []corev1.HTTPHeader{authHeader}
}

func getEsAssociations(params Params) []commonv1.Association {
	var esAssociations []commonv1.Association

	for _, assoc := range params.Logstash.GetAssociations() {
		if assoc.AssociationType() == commonv1.ElasticsearchAssociationType {
			esAssociations = append(esAssociations, assoc)
		}
	}
	return esAssociations
}

func writeEsAssocToConfigHash(params Params, esAssociations []commonv1.Association, configHash hash.Hash) error {
	if esAssociations == nil {
		return nil
	}

	return commonassociation.WriteAssocsToConfigHash(
		params.Client,
		esAssociations,
		configHash,
	)
}

func getHTTPSInternalCertsSecret(params Params) (corev1.Secret, error) {
	var httpCerts corev1.Secret

	err := params.Client.Get(params.Context, types.NamespacedName{
		Namespace: params.Logstash.Namespace,
		Name:      certificates.InternalCertsSecretName(logstashv1alpha1.Namer, params.Logstash.Name),
	}, &httpCerts)

	if err != nil {
		return httpCerts, err
	}

	return httpCerts, nil
}

// writeHTTPSCertsToConfigHash fetches the http-certs-internal secret and adds the content of tls.crt to checksum
func writeHTTPSCertsToConfigHash(params Params, configHash hash.Hash) error {
	if params.APIServerConfig.UseTLS() {
		httpCerts, err := getHTTPSInternalCertsSecret(params)
		if err != nil {
			return err
		}

		if httpCert, ok := httpCerts.Data[certificates.CertFileName]; ok {
			_, _ = configHash.Write(httpCert)
		}
	}

	return nil
}
