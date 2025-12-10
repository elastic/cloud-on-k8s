// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"fmt"
	"strconv"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/go-ucfg"

	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/pod"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/initcontainer"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/network"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/stackmon"
	kbvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/volume"
)

const (
	defaultFSGroup = 1000
	defaultFSUser  = 1000
	// basePathEnvName is the environment variable name that allows ibe to specify a path to mount Kibana at if you are running behind a proxy
	basePathEnvName = "SERVER_BASEPATH"
	// rewriteBasePathEnvName is the environment variable name that specifies whether Kibana should rewrite requests that are prefixed with server.basePath
	rewriteBasePathEnvName = "SERVER_REWRITEBASEPATH"
)

var (
	// DataVolume is used to propagate the keystore file from the init container to
	// Kibana running in the main container.
	// Since Kibana is stateless and the keystore is created on pod start, an EmptyDir is fine here.
	DataVolume = volume.NewEmptyDirVolume(kbvolume.DataVolumeName, kbvolume.DataVolumeMountPath)

	// PluginsVolume can be used to persist plugins after installation via an init container when
	// the Kibana pod has readOnlyRootFilesystem set to true.
	PluginsVolume = volume.NewEmptyDirVolume(kbvolume.PluginsVolumeName, kbvolume.PluginsVolumeMountPath)

	// LogsVolume can be used to persist logs even when
	// the Kibana pod has readOnlyRootFilesystem set to true.
	LogsVolume = volume.NewEmptyDirVolume(kbvolume.LogsVolumeName, kbvolume.LogsVolumeMountPath)

	// TempVolume can be used for some reporting features when the Kibana pod has
	// readOnlyRootFilesystem set to true.
	TempVolume = volume.NewEmptyDirVolume(kbvolume.TempVolumeName, kbvolume.TempVolumeMountPath)

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

// basePathConfig is used to get the base path from the Kibana configuration.
type basePathConfig struct {
	Server struct {
		RewriteBasePath bool   `config:"rewriteBasePath"`
		BasePath        string `config:"basePath"`
	}
}

// readinessProbe is the readiness probe for the Kibana container
func readinessProbe(useTLS bool, basePath string) corev1.Probe {
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
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(network.HTTPPort),
				Path:   fmt.Sprintf("%s/login", basePath),
				Scheme: scheme,
			},
		},
	}
}

func NewPodTemplateSpec(
	ctx context.Context,
	client k8sclient.Client,
	kb kbv1.Kibana,
	keystore *keystore.Resources,
	volumes []volume.VolumeLike,
	basePath string,
	setDefaultSecurityContext bool,
	meta metadata.Metadata,
) (corev1.PodTemplateSpec, error) {
	labels := kb.GetIdentityLabels()
	labels[kblabel.KibanaVersionLabelName] = kb.Spec.Version

	ports := getDefaultContainerPorts(kb)
	v, err := version.Parse(kb.Spec.Version)
	if err != nil {
		return corev1.PodTemplateSpec{}, err // error unlikely and should have been caught during validation
	}

	scriptsConfigMapVolume := initcontainer.NewScriptsConfigMapVolume(kb.Name)
	meta = meta.Merge(metadata.Metadata{
		Labels:      labels,
		Annotations: DefaultAnnotations,
	})
	builder := defaults.NewPodTemplateBuilder(kb.Spec.PodTemplate, kbv1.KibanaContainerName).
		WithResources(DefaultResources).
		WithLabels(meta.Labels).
		WithAnnotations(meta.Annotations).
		WithDockerImage(kb.Spec.Image, container.ImageRepository(container.KibanaImage, v)).
		WithReadinessProbe(readinessProbe(kb.Spec.HTTP.TLS.Enabled(), basePath)).
		WithVolumes(scriptsConfigMapVolume.Volume()).WithVolumeMounts(scriptsConfigMapVolume.VolumeMount()).
		WithVolumes(PluginsVolume.Volume()).WithVolumeMounts(PluginsVolume.VolumeMount()).
		WithPorts(ports).
		// Temporary fix to expand the usable memory to 75% of the total memory for Kibana until https://github.com/elastic/kibana/issues/245742 is implemented.
		WithEnv(corev1.EnvVar{Name: EnvNodeOptions, Value: "--max-old-space-size-percentage=75"})

	for _, volume := range volumes {
		builder.WithVolumes(volume.Volume()).WithVolumeMounts(volume.VolumeMount())
	}

	// Kibana 7.5.0 and above support running with a read-only root filesystem,
	// but require a temporary volume to be mounted at /tmp for some reporting features
	// and a plugin volume mounted at /usr/share/kibana/plugins. Also needed is an
	// init container to copy any existing plugins in /usr/share/kibana/plugins to the
	// temporary volume.
	// Limiting to 7.10.0 here as there was a bug in previous versions causing rebuilding
	// of browser bundles to happen on plugin install, which would attempt a write to the
	// root filesystem on restart.
	var canEnableSecurityContext = v.GTE(initcontainer.HardenedSecurityContextSupportedVersion) && setDefaultSecurityContext
	if canEnableSecurityContext {
		builder.WithContainersSecurityContext(defaultSecurityContext).
			WithPodSecurityContext(defaultPodSecurityContext).
			WithVolumes(LogsVolume.Volume()).WithVolumeMounts(LogsVolume.VolumeMount()).
			WithVolumes(TempVolume.Volume()).WithVolumeMounts(TempVolume.VolumeMount())
	}

	initContainer, err := initcontainer.NewInitContainer(kb, setDefaultSecurityContext)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	builder.WithInitContainers(initContainer)

	if keystore != nil {
		builder.WithVolumes(keystore.Volume).
			WithInitContainers(keystore.InitContainer)
	}

	builder, err = stackmon.WithMonitoring(ctx, client, builder, kb, basePath, meta)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	return builder.WithInitContainerDefaults().PodTemplate, nil
}

// GetKibanaContainer returns the Kibana container from the given podSpec.
func GetKibanaContainer(podSpec corev1.PodSpec) *corev1.Container {
	return pod.ContainerByName(podSpec, kbv1.KibanaContainerName)
}

func GetKibanaBasePathFromSpecEnv(podSpec corev1.PodSpec) (string, error) {
	kbContainer := GetKibanaContainer(podSpec)
	if kbContainer == nil {
		return "", nil
	}

	envMap := make(map[string]string)
	for _, envVar := range kbContainer.Env {
		if envVar.Name == basePathEnvName || envVar.Name == rewriteBasePathEnvName {
			envMap[envVar.Name] = envVar.Value
		}
	}

	// If SERVER_REWRITEBASEPATH is set to true, we should use the value of SERVER_BASEPATH
	if rewriteBasePath, ok := envMap[rewriteBasePathEnvName]; ok {
		rewriteBasePathBool, err := strconv.ParseBool(rewriteBasePath)
		if err != nil {
			return "", fmt.Errorf("failed to parse SERVER_REWRITEBASEPATH value %s: %w", rewriteBasePath, err)
		}
		if rewriteBasePathBool {
			return envMap[basePathEnvName], nil
		}
	}

	return "", nil
}

func getDefaultContainerPorts(kb kbv1.Kibana) []corev1.ContainerPort {
	return []corev1.ContainerPort{{Name: kb.Spec.HTTP.Protocol(), ContainerPort: int32(network.HTTPPort), Protocol: corev1.ProtocolTCP}}
}

func GetKibanaBasePath(kb kbv1.Kibana) (string, error) {
	// We only support the case where both base path and rewrite base path are set in the ENV or the config
	// We will not support the case where base path is set in the ENV and rewrite base path is set in the config or vice versa
	kbBasePath, err := GetKibanaBasePathFromSpecEnv(kb.Spec.PodTemplate.Spec)
	if err != nil {
		return "", err
	}

	if kbBasePath != "" {
		return kbBasePath, nil
	}

	if kb.Spec.Config == nil {
		return "", nil
	}

	kbucfgConfig, err := ucfg.NewFrom(kb.Spec.Config.Data, settings.Options...)
	if err != nil {
		return "", err
	}

	kbCfg := basePathConfig{}
	if err := kbucfgConfig.Unpack(&kbCfg); err != nil {
		return "", err
	}

	if kbCfg.Server.RewriteBasePath {
		return kbCfg.Server.BasePath, nil
	}

	return "", nil
}
