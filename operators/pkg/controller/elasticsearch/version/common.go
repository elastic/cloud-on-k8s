// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version

import (
	"path"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var (
	DefaultMemoryLimits = resource.MustParse("2Gi")
	SecurityPropsFile   = path.Join(settings.ManagedConfigPath, settings.SecurityPropsFile)
)

// NewExpectedPodSpecs creates PodSpecContexts for all Elasticsearch nodes in the given Elasticsearch cluster
func NewExpectedPodSpecs(
	es v1alpha1.Elasticsearch,
	paramsTmpl pod.NewPodSpecParams,
	newEnvironmentVarsFn func(p pod.NewPodSpecParams, heapSize int, certs, creds, securecommon volume.SecretVolume) []corev1.EnvVar,
	newESConfigFn func(clusterName string, config v1alpha1.Config) (settings.CanonicalConfig, error),
	newInitContainersFn func(imageName string, operatorImage string, setVMMaxMapCount *bool, transportCerts volume.SecretVolume) ([]corev1.Container, error),
	operatorImage string,
) ([]pod.PodSpecContext, error) {
	podSpecs := make([]pod.PodSpecContext, 0, es.Spec.NodeCount())

	for _, node := range es.Spec.Nodes {
		for i := int32(0); i < node.NodeCount; i++ {
			params := pod.NewPodSpecParams{
				// cluster-wide params
				Version:          es.Spec.Version,
				CustomImageName:  es.Spec.Image,
				ClusterName:      es.Name,
				SetVMMaxMapCount: es.Spec.SetVMMaxMapCount,
				// volumes
				UsersSecretVolume:  paramsTmpl.UsersSecretVolume,
				ConfigMapVolume:    paramsTmpl.ConfigMapVolume,
				ClusterSecretsRef:  paramsTmpl.ClusterSecretsRef,
				ProbeUser:          paramsTmpl.ProbeUser,
				ReloadCredsUser:    paramsTmpl.ReloadCredsUser,
				UnicastHostsVolume: paramsTmpl.UnicastHostsVolume,
				// pod params
				NodeSpec: node,
			}
			podSpec, config, err := podSpec(
				params,
				operatorImage,
				newEnvironmentVarsFn,
				newESConfigFn,
				newInitContainersFn,
			)
			if err != nil {
				return nil, err
			}

			podSpecs = append(podSpecs, pod.PodSpecContext{PodSpec: podSpec, NodeSpec: node, Config: config})
		}
	}

	return podSpecs, nil
}

// podSpec creates a new PodSpec for an Elasticsearch node
func podSpec(
	p pod.NewPodSpecParams,
	operatorImage string,
	newEnvironmentVarsFn func(p pod.NewPodSpecParams, heapSize int, certs, creds, keystore volume.SecretVolume) []corev1.EnvVar,
	newESConfigFn func(clusterName string, config v1alpha1.Config) (settings.CanonicalConfig, error),
	newInitContainersFn func(elasticsearchImage string, operatorImage string, setVMMaxMapCount *bool, transportCerts volume.SecretVolume) ([]corev1.Container, error),
) (corev1.PodSpec, settings.CanonicalConfig, error) {
	// setup volumes
	probeSecret := volume.NewSelectiveSecretVolumeWithMountPath(
		user.ElasticInternalUsersSecretName(p.ClusterName), volume.ProbeUserVolumeName,
		volume.ProbeUserSecretMountPath, []string{p.ProbeUser.Name},
	)
	reloadCredsSecret := volume.NewSelectiveSecretVolumeWithMountPath(
		user.ElasticInternalUsersSecretName(p.ClusterName), volume.ReloadCredsUserVolumeName,
		volume.ReloadCredsUserSecretMountPath, []string{p.ReloadCredsUser.Name},
	)
	clusterSecretsSecretVolume := volume.NewSecretVolumeWithMountPath(
		p.ClusterSecretsRef.Name,
		"secrets",
		volume.ClusterSecretsVolumeMountPath,
	)
	// we don't have a secret name for this, this will be injected as a volume for us upon creation, this is fine
	// because we will not be adding this to the container Volumes, only the VolumeMounts section.
	transportCertificatesVolume := volume.NewSecretVolumeWithMountPath(
		"",
		volume.TransportCertificatesSecretVolumeName,
		volume.TransportCertificatesSecretVolumeMountPath,
	)
	secureSettingsVolume := volume.NewSecretVolumeWithMountPath(
		name.SecureSettingsSecret(p.ClusterName),
		volume.SecureSettingsVolumeName,
		volume.SecureSettingsVolumeMountPath,
	)
	httpCertificatesVolume := volume.NewSecretVolumeWithMountPath(
		name.HTTPCertsInternalSecretName(p.ClusterName),
		volume.HTTPCertificatesSecretVolumeName,
		volume.HTTPCertificatesSecretVolumeMountPath,
	)

	// build on top of the user-provided pod template spec
	builder := defaults.NewPodTemplateBuilder(p.NodeSpec.PodTemplate, v1alpha1.ElasticsearchContainerName).
		WithDockerImage(p.CustomImageName, stringsutil.Concat(pod.DefaultImageRepository, ":", p.Version)).
		WithTerminationGracePeriod(pod.DefaultTerminationGracePeriodSeconds).
		WithPorts(pod.DefaultContainerPorts).
		WithReadinessProbe(*pod.NewReadinessProbe()).
		WithCommand([]string{processmanager.CommandPath}).
		// enforce a memory resource limits if not provided by the user, since we need to compute JVM heap size
		// we do not set resource Requests here in order to end up in the qosClass of Guaranteed by default
		// see https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/ for more details
		WithMemoryLimit(DefaultMemoryLimits)

	// setup heap size based on memory limits
	heapSize := MemoryLimitsToHeapSize(*builder.Container.Resources.Limits.Memory())
	builder = builder.WithEnv(newEnvironmentVarsFn(p, heapSize, httpCertificatesVolume, reloadCredsSecret, secureSettingsVolume)...)

	// setup init containers
	initContainers, err := newInitContainersFn(builder.Container.Image, operatorImage, p.SetVMMaxMapCount, transportCertificatesVolume)
	if err != nil {
		return corev1.PodSpec{}, settings.CanonicalConfig{}, err
	}

	builder = builder.
		WithVolumes(
			append(initcontainer.PrepareFsSharedVolumes.Volumes(),
				initcontainer.ProcessManagerVolume.Volume(),
				p.UsersSecretVolume.Volume(),
				p.ConfigMapVolume.Volume(),
				p.UnicastHostsVolume.Volume(),
				probeSecret.Volume(),
				clusterSecretsSecretVolume.Volume(),
				reloadCredsSecret.Volume(),
				secureSettingsVolume.Volume(),
				httpCertificatesVolume.Volume(),
			)...).
		WithVolumeMounts(
			append(initcontainer.PrepareFsSharedVolumes.EsContainerVolumeMounts(),
				initcontainer.ProcessManagerVolume.EsContainerVolumeMount(),
				p.UsersSecretVolume.VolumeMount(),
				p.ConfigMapVolume.VolumeMount(),
				p.UnicastHostsVolume.VolumeMount(),
				probeSecret.VolumeMount(),
				clusterSecretsSecretVolume.VolumeMount(),
				transportCertificatesVolume.VolumeMount(),
				reloadCredsSecret.VolumeMount(),
				secureSettingsVolume.VolumeMount(),
				httpCertificatesVolume.VolumeMount(),
			)...).
		WithInitContainers(initContainers...)

	// generate the configuration
	// actual volumes to propagate it will be created later on
	config := p.NodeSpec.Config
	if config == nil {
		config = &v1alpha1.Config{}
	}
	esConfig, err := newESConfigFn(p.ClusterName, *config)
	if err != nil {
		return corev1.PodSpec{}, settings.CanonicalConfig{}, err
	}

	return builder.PodTemplate.Spec, esConfig, nil
}

// NewPod constructs a pod from the given parameters.
func NewPod(
	version version.Version,
	es v1alpha1.Elasticsearch,
	podSpecCtx pod.PodSpecContext,
) (corev1.Pod, error) {
	// build on top of user-provided objectMeta to reuse labels, annotations, etc.
	builder := defaults.NewPodTemplateBuilder(podSpecCtx.NodeSpec.PodTemplate, v1alpha1.ElasticsearchContainerName)

	// set our own name & namespace
	builder.PodTemplate.Name = name.NewPodName(es.Name, podSpecCtx.NodeSpec)
	builder.PodTemplate.Namespace = es.Namespace

	cfg, err := podSpecCtx.Config.Unpack()
	if err != nil {
		return corev1.Pod{}, err
	}

	builder = builder.WithLabels(label.NewPodLabels(es, version, cfg))

	if podSpecCtx.PodSpec.Hostname == "" {
		podSpecCtx.PodSpec.Hostname = builder.PodTemplate.Name
	}

	if podSpecCtx.PodSpec.Subdomain == "" {
		podSpecCtx.PodSpec.Subdomain = es.Name
	}

	return corev1.Pod{
		ObjectMeta: builder.PodTemplate.ObjectMeta,
		Spec:       podSpecCtx.PodSpec,
	}, nil
}

// MemoryLimitsToHeapSize converts a memory limit to the heap size (in megabytes) for the JVM
func MemoryLimitsToHeapSize(memoryLimit resource.Quantity) int {
	// use half the available memory as heap
	return quantityToMegabytes(memoryLimit) / 2
}

// quantityToMegabytes returns the megabyte value of the provided resource.Quantity
func quantityToMegabytes(q resource.Quantity) int {
	return int(q.Value()) / 1024 / 1024
}
