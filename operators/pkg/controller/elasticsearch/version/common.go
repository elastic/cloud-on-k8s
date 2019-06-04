// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version

import (
	"path"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/overrides"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/services"
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
	newEnvironmentVarsFn func(p pod.NewPodSpecParams, heapSize int, certs, creds, secureSettings volume.SecretVolume) []corev1.EnvVar,
	newESConfigFn func(clusterName string, config v1alpha1.Config) (*settings.CanonicalConfig, error),
	newInitContainersFn func(imageName string, operatorImage string, setVMMaxMapCount *bool, transportCerts volume.SecretVolume) ([]corev1.Container, error),
	operatorImage string,
) ([]pod.PodSpecContext, error) {
	podSpecs := make([]pod.PodSpecContext, 0, es.Spec.NodeCount())

	for _, node := range es.Spec.Nodes {
		for i := int32(0); i < node.NodeCount; i++ {
			params := pod.NewPodSpecParams{
				// cluster-wide params
				Version:              es.Spec.Version,
				CustomImageName:      es.Spec.Image,
				ClusterName:          es.Name,
				DiscoveryServiceName: services.DiscoveryServiceName(es.Name),
				SetVMMaxMapCount:     es.Spec.SetVMMaxMapCount,
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
	newESConfigFn func(clusterName string, config v1alpha1.Config) (*settings.CanonicalConfig, error),
	newInitContainersFn func(elasticsearchImage string, operatorImage string, setVMMaxMapCount *bool, transportCerts volume.SecretVolume) ([]corev1.Container, error),
) (corev1.PodSpec, *settings.CanonicalConfig, error) {
	// build on top of the user-provided pod template spec
	podSpec := p.NodeSpec.PodTemplate.Spec.DeepCopy()

	// build image name from version, or use custom user-provided one
	image := stringsutil.Concat(pod.DefaultImageRepository, ":", p.Version)
	if p.CustomImageName != "" {
		image = p.CustomImageName
	}

	// override pod spec fields with our defaults if not provided by the user
	if podSpec.TerminationGracePeriodSeconds == nil {
		period := pod.DefaultTerminationGracePeriodSeconds
		podSpec.TerminationGracePeriodSeconds = &period
	}
	if podSpec.AutomountServiceAccountToken == nil {
		automountSA := false
		podSpec.AutomountServiceAccountToken = &automountSA
	}

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

	// append our volumes to user-provided ones
	podSpec.Volumes = append(
		podSpec.Volumes,
		append(
			initcontainer.PrepareFsSharedVolumes.Volumes(),
			initcontainer.PrivateKeySharedVolume.Volume(),
			initcontainer.ProcessManagerVolume.Volume(),
			p.UsersSecretVolume.Volume(),
			p.ConfigMapVolume.Volume(),
			p.UnicastHostsVolume.Volume(),
			probeSecret.Volume(),
			clusterSecretsSecretVolume.Volume(),
			reloadCredsSecret.Volume(),
			secureSettingsVolume.Volume(),
			httpCertificatesVolume.Volume(),
		)...,
	)

	// append out init containers to user-provided ones
	initContainers, err := newInitContainersFn(image, operatorImage, p.SetVMMaxMapCount, transportCertificatesVolume)
	if err != nil {
		return corev1.PodSpec{}, nil, err
	}
	podSpec.InitContainers = append(podSpec.InitContainers, initContainers...)

	// build on top of the user-provided ES container spec, or create a new one
	containerSpec := p.NodeSpec.GetESContainerTemplate().DeepCopy()
	userProvidedContainerSpec := containerSpec != nil
	if !userProvidedContainerSpec {
		containerSpec = &corev1.Container{
			Name: v1alpha1.ElasticsearchContainerName,
		}
	}

	// set memory resource limits if not provided by the user
	containerSpec.Resources.Limits = buildResourceLimits(containerSpec)
	// we do not override resource Requests here in order to end up in the qosClass of Guaranteed by default
	// see https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/ for more details

	heapSize := MemoryLimitsToHeapSize(*containerSpec.Resources.Limits.Memory())
	// inherit user-provided environment...
	envBuilder := overrides.NewEnvBuilder(containerSpec.Env...)
	// ...that we augment with our own.
	// if a user-provided var has the same name as one of ours, we keep the user's version.
	// this may break the deployment, but we consider users know what they are doing at this point.
	envBuilder.AddIfMissing(newEnvironmentVarsFn(p, heapSize, httpCertificatesVolume, reloadCredsSecret, secureSettingsVolume)...)
	containerSpec.Env = envBuilder.GetEnvVars()

	// set the container image to our own if not provided by the user
	if containerSpec.Image == "" {
		containerSpec.Image = image
	}
	// ImagePullPolicy is kept either user-provided or defaulted

	// override ports
	containerSpec.Ports = pod.DefaultContainerPorts

	// override readiness probe
	containerSpec.ReadinessProbe = pod.NewReadinessProbe()

	// append our volume mounts to user-provided ones
	containerSpec.VolumeMounts = append(
		containerSpec.VolumeMounts,
		append(
			initcontainer.PrepareFsSharedVolumes.EsContainerVolumeMounts(),
			[]corev1.VolumeMount{
				initcontainer.PrivateKeySharedVolume.EsContainerVolumeMount(),
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
			}...,
		)...,
	)

	// override command
	containerSpec.Command = []string{processmanager.CommandPath}

	// set the container spec back into the podSpec container list
	if userProvidedContainerSpec {
		// replace existing one
		for i, c := range podSpec.Containers {
			if c.Name == v1alpha1.ElasticsearchContainerName {
				podSpec.Containers[i] = *containerSpec
			}
		}
	} else {
		podSpec.Containers = append(podSpec.Containers, *containerSpec)
	}

	// generate the configuration
	// actual volumes to propagate it will be created later on
	config := p.NodeSpec.Config
	if config == nil {
		config = &v1alpha1.Config{}
	}
	esConfig, err := newESConfigFn(p.ClusterName, *config)
	if err != nil {
		return corev1.PodSpec{}, nil, err
	}

	return *podSpec, esConfig, nil
}

// NewPod constructs a pod from the given parameters.
func NewPod(
	version version.Version,
	es v1alpha1.Elasticsearch,
	podSpecCtx pod.PodSpecContext,
) (corev1.Pod, error) {
	// build on top of user-provided objectMeta to reuse labels, annotations, etc.
	objectMeta := podSpecCtx.NodeSpec.PodTemplate.ObjectMeta

	// set our own name & namespace
	objectMeta.Name = name.NewPodName(es.Name, podSpecCtx.NodeSpec)
	objectMeta.Namespace = es.Namespace

	// build labels on top of user-provided ones
	if objectMeta.Labels == nil {
		objectMeta.Labels = map[string]string{}
	}
	cfg, err := podSpecCtx.Config.Unpack()
	if err != nil {
		return corev1.Pod{}, err
	}
	for k, v := range label.NewPodLabels(es, version, cfg) {
		// don't override user-provided labels
		// this may lead to issues but we consider users know what they are doing at this point.
		if _, exists := objectMeta.Labels[k]; !exists {
			objectMeta.Labels[k] = v
		}
	}

	if podSpecCtx.PodSpec.Hostname == "" {
		podSpecCtx.PodSpec.Hostname = objectMeta.Name
	}

	if podSpecCtx.PodSpec.Subdomain == "" {
		podSpecCtx.PodSpec.Subdomain = es.Name
	}

	return corev1.Pod{
		ObjectMeta: objectMeta,
		Spec:       podSpecCtx.PodSpec,
	}, nil
}

// MemoryLimitsToHeapSize converts a memory limit to the heap size (in megabytes) for the JVM
func MemoryLimitsToHeapSize(memoryLimit resource.Quantity) int {
	// use half the available memory as heap
	return quantityToMegabytes(nonZeroQuantityOrDefault(memoryLimit, DefaultMemoryLimits)) / 2
}

// nonZeroQuantityOrDefault returns q if it is nonzero, defaultQuantity otherwise
func nonZeroQuantityOrDefault(q, defaultQuantity resource.Quantity) resource.Quantity {
	if q.IsZero() {
		return defaultQuantity
	}
	return q
}

// quantityToMegabytes returns the megabyte value of the provided resource.Quantity
func quantityToMegabytes(q resource.Quantity) int {
	return int(q.Value()) / 1024 / 1024
}

func buildResourceLimits(esContainer *corev1.Container) corev1.ResourceList {
	resourceLimits := corev1.ResourceList{}
	if esContainer != nil && esContainer.Resources.Limits != nil {
		resourceLimits = esContainer.Resources.Limits
	}
	resourceLimits[corev1.ResourceMemory] = nonZeroQuantityOrDefault(*resourceLimits.Memory(), DefaultMemoryLimits)
	return resourceLimits
}
