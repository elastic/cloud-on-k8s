// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/user"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
)

// NewExpectedPodSpecs creates PodSpecContexts for all Elasticsearch nodes in the given Elasticsearch cluster
func NewExpectedPodSpecs(
	es v1alpha1.Elasticsearch,
	paramsTmpl pod.NewPodSpecParams,
	newEnvironmentVarsFn func(p pod.NewPodSpecParams, certs, creds, securecommon volume.SecretVolume) []corev1.EnvVar,
	newESConfigFn func(clusterName string, config commonv1alpha1.Config) (settings.CanonicalConfig, error),
	newInitContainersFn func(imageName string, operatorImage string, setVMMaxMapCount *bool, transportCerts volume.SecretVolume, clusterName string) ([]corev1.Container, error),
	operatorImage string,
) ([]pod.PodSpecContext, error) {
	podSpecs := make([]pod.PodSpecContext, 0, es.Spec.NodeCount())

	for _, node := range es.Spec.Nodes {
		// add default PVCs to the node spec
		node.VolumeClaimTemplates = defaults.AppendDefaultPVCs(
			node.VolumeClaimTemplates, node.PodTemplate.Spec, esvolume.DefaultVolumeClaimTemplates...,
		)

		for i := int32(0); i < node.NodeCount; i++ {
			params := pod.NewPodSpecParams{
				// cluster-wide params
				Elasticsearch: es,
				// volumes
				UsersSecretVolume:  paramsTmpl.UsersSecretVolume,
				ProbeUser:          paramsTmpl.ProbeUser,
				KeystoreUser:       paramsTmpl.KeystoreUser,
				UnicastHostsVolume: paramsTmpl.UnicastHostsVolume,
				// pod params
				NodeSpec: node,
			}
			podSpecCtx, err := podSpecContext(
				params,
				operatorImage,
				newEnvironmentVarsFn,
				newESConfigFn,
				newInitContainersFn,
			)
			if err != nil {
				return nil, err
			}

			podSpecs = append(podSpecs, podSpecCtx)
		}
	}

	return podSpecs, nil
}

// podSpecContext creates a new PodSpecContext for an Elasticsearch node
func podSpecContext(
	p pod.NewPodSpecParams,
	operatorImage string,
	newEnvironmentVarsFn func(p pod.NewPodSpecParams, certs, creds, keystore volume.SecretVolume) []corev1.EnvVar,
	newESConfigFn func(clusterName string, config commonv1alpha1.Config) (settings.CanonicalConfig, error),
	newInitContainersFn func(elasticsearchImage string, operatorImage string, setVMMaxMapCount *bool, transportCerts volume.SecretVolume, clusterName string) ([]corev1.Container, error),
) (pod.PodSpecContext, error) {
	// setup volumes
	probeSecret := volume.NewSelectiveSecretVolumeWithMountPath(
		user.ElasticInternalUsersSecretName(p.Elasticsearch.Name), esvolume.ProbeUserVolumeName,
		esvolume.ProbeUserSecretMountPath, []string{p.ProbeUser.Name},
	)
	keystoreUserSecret := volume.NewSelectiveSecretVolumeWithMountPath(
		user.ElasticInternalUsersSecretName(p.Elasticsearch.Name), esvolume.KeystoreUserVolumeName,
		esvolume.KeystoreUserSecretMountPath, []string{p.KeystoreUser.Name},
	)
	// we don't have a secret name for this, this will be injected as a volume for us upon creation, this is fine
	// because we will not be adding this to the container Volumes, only the VolumeMounts section.
	transportCertificatesVolume := volume.NewSecretVolumeWithMountPath(
		"",
		esvolume.TransportCertificatesSecretVolumeName,
		esvolume.TransportCertificatesSecretVolumeMountPath,
	)
	secureSettingsVolume := volume.NewSecretVolumeWithMountPath(
		name.SecureSettingsSecret(p.Elasticsearch.Name),
		esvolume.SecureSettingsVolumeName,
		esvolume.SecureSettingsVolumeMountPath,
	)
	httpCertificatesVolume := volume.NewSecretVolumeWithMountPath(
		name.HTTPCertsInternalSecretName(p.Elasticsearch.Name),
		esvolume.HTTPCertificatesSecretVolumeName,
		esvolume.HTTPCertificatesSecretVolumeMountPath,
	)

	// build on top of the user-provided pod template spec
	builder := defaults.NewPodTemplateBuilder(p.NodeSpec.PodTemplate, v1alpha1.ElasticsearchContainerName).
		WithDockerImage(p.Elasticsearch.Spec.Image, stringsutil.Concat(pod.DefaultImageRepository, ":", p.Elasticsearch.Spec.Version)).
		WithTerminationGracePeriod(pod.DefaultTerminationGracePeriodSeconds).
		WithPorts(pod.DefaultContainerPorts).
		WithReadinessProbe(*pod.NewReadinessProbe()).
		WithCommand([]string{processmanager.CommandPath}).
		WithAffinity(pod.DefaultAffinity(p.Elasticsearch.Name)).
		WithEnv(newEnvironmentVarsFn(p, httpCertificatesVolume, keystoreUserSecret, secureSettingsVolume)...)

	// setup init containers
	initContainers, err := newInitContainersFn(
		builder.Container.Image,
		operatorImage,
		p.Elasticsearch.Spec.SetVMMaxMapCount,
		transportCertificatesVolume,
		p.Elasticsearch.Name)

	if err != nil {
		return pod.PodSpecContext{}, err
	}

	scriptsVolume := volume.NewConfigMapVolumeWithMode(
		name.ScriptsConfigMap(p.Elasticsearch.Name),
		esvolume.ScriptsVolumeName,
		esvolume.ScriptsVolumeMountPath,
		0755)

	builder = builder.
		WithVolumes(
			append(initcontainer.PrepareFsSharedVolumes.Volumes(),
				initcontainer.ProcessManagerVolume.Volume(),
				p.UsersSecretVolume.Volume(),
				p.UnicastHostsVolume.Volume(),
				probeSecret.Volume(),
				keystoreUserSecret.Volume(),
				secureSettingsVolume.Volume(),
				httpCertificatesVolume.Volume(),
				scriptsVolume.Volume(),
			)...).
		WithVolumeMounts(
			append(initcontainer.PrepareFsSharedVolumes.EsContainerVolumeMounts(),
				initcontainer.ProcessManagerVolume.EsContainerVolumeMount(),
				p.UsersSecretVolume.VolumeMount(),
				p.UnicastHostsVolume.VolumeMount(),
				probeSecret.VolumeMount(),
				transportCertificatesVolume.VolumeMount(),
				keystoreUserSecret.VolumeMount(),
				secureSettingsVolume.VolumeMount(),
				httpCertificatesVolume.VolumeMount(),
				scriptsVolume.VolumeMount(),
			)...).
		WithInitContainerDefaults().
		WithInitContainers(initContainers...)

	// generate the configuration
	// actual volumes to propagate it will be created later on
	config := p.NodeSpec.Config
	if config == nil {
		config = &commonv1alpha1.Config{}
	}
	esConfig, err := newESConfigFn(p.Elasticsearch.Name, *config)
	if err != nil {
		return pod.PodSpecContext{}, err
	}
	unpackedCfg, err := esConfig.Unpack()
	if err != nil {
		return pod.PodSpecContext{}, err
	}

	// set labels
	version, err := version.Parse(p.Elasticsearch.Spec.Version)
	if err != nil {
		return pod.PodSpecContext{}, err
	}
	builder = builder.WithLabels(label.NewPodLabels(p.Elasticsearch, *version, unpackedCfg))

	return pod.PodSpecContext{
		NodeSpec: p.NodeSpec,
		PodSpec:  builder.PodTemplate.Spec,
		Labels:   builder.PodTemplate.Labels,
		Config:   esConfig,
	}, nil
}

// NewPod constructs a pod from the given parameters.
func NewPod(
	es v1alpha1.Elasticsearch,
	podSpecCtx pod.PodSpecContext,
) (corev1.Pod, error) {
	// build on top of user-provided objectMeta to reuse labels, annotations, etc.
	builder := defaults.NewPodTemplateBuilder(podSpecCtx.NodeSpec.PodTemplate, v1alpha1.ElasticsearchContainerName)

	// set our own name & namespace
	builder.PodTemplate.Name = name.NewPodName(es.Name, podSpecCtx.NodeSpec)
	builder.PodTemplate.Namespace = es.Namespace
	// apply labels computed in the podSpecCtx
	builder.PodTemplate.Labels = podSpecCtx.Labels

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

// quantityToMegabytes returns the megabyte value of the provided resource.Quantity
func quantityToMegabytes(q resource.Quantity) int {
	return int(q.Value()) / 1024 / 1024
}
