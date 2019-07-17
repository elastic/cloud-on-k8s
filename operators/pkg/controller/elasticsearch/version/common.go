// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version

import (
	"crypto/sha256"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/user"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
)

var (
	// DefaultResources for the Elasticsearch container. The JVM default heap size is 1Gi, so we
	// request at least 2Gi for the container to make sure ES can work properly.
	// Not applying this minimum default would make ES randomly crash (OOM) on small machines.
	DefaultResources = corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
)

// TODO: refactor
type PodTemplateSpecBuilder func(v1alpha1.NodeSpec, settings.CanonicalConfig) (corev1.PodTemplateSpec, error)

// TODO: refactor to avoid all the params mess
func BuildPodTemplateSpec(
	es v1alpha1.Elasticsearch,
	nodeSpec v1alpha1.NodeSpec,
	paramsTmpl pod.NewPodSpecParams,
	cfg settings.CanonicalConfig,
	newEnvironmentVarsFn func(p pod.NewPodSpecParams) []corev1.EnvVar,
	newInitContainersFn func(imageName string, setVMMaxMapCount *bool, transportCerts volume.SecretVolume, clusterName string) ([]corev1.Container, error),
) (corev1.PodTemplateSpec, error) {
	params := pod.NewPodSpecParams{
		// cluster-wide params
		Elasticsearch: es,
		// volumes
		UsersSecretVolume:  paramsTmpl.UsersSecretVolume,
		ProbeUser:          paramsTmpl.ProbeUser,
		UnicastHostsVolume: paramsTmpl.UnicastHostsVolume,
		// volume and init container for the keystore
		KeystoreResources: paramsTmpl.KeystoreResources,
		// pod params
		NodeSpec: nodeSpec,
	}
	podSpecCtx, err := podSpecContext(
		params,
		cfg,
		newEnvironmentVarsFn,
		newInitContainersFn,
	)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	return podSpecCtx.PodTemplate, nil
}

// podSpecContext creates a new PodSpecContext for an Elasticsearch node
func podSpecContext(
	p pod.NewPodSpecParams,
	cfg settings.CanonicalConfig,
	newEnvironmentVarsFn func(p pod.NewPodSpecParams) []corev1.EnvVar,
	newInitContainersFn func(elasticsearchImage string, setVMMaxMapCount *bool, transportCerts volume.SecretVolume, clusterName string) ([]corev1.Container, error),
) (pod.PodSpecContext, error) {
	statefulSetName := name.StatefulSet(p.Elasticsearch.Name, p.NodeSpec.Name)

	// setup volumes
	probeSecret := volume.NewSelectiveSecretVolumeWithMountPath(
		user.ElasticInternalUsersSecretName(p.Elasticsearch.Name), esvolume.ProbeUserVolumeName,
		esvolume.ProbeUserSecretMountPath, []string{p.ProbeUser.Name},
	)
	httpCertificatesVolume := volume.NewSecretVolumeWithMountPath(
		certificates.HTTPCertsInternalSecretName(name.ESNamer, p.Elasticsearch.Name),
		esvolume.HTTPCertificatesSecretVolumeName,
		esvolume.HTTPCertificatesSecretVolumeMountPath,
	)
	transportCertificatesVolume := volume.NewSecretVolumeWithMountPath(
		name.TransportCertificatesSecret(p.Elasticsearch.Name),
		esvolume.TransportCertificatesSecretVolumeName,
		esvolume.TransportCertificatesSecretVolumeMountPath,
	)

	ssetName := name.StatefulSet(p.Elasticsearch.Name, p.NodeSpec.Name)
	configVolume := settings.ConfigSecretVolume(ssetName)

	// append future volumes from PVCs (not resolved to a claim yet)
	persistentVolumes := make([]corev1.Volume, 0, len(p.NodeSpec.VolumeClaimTemplates))
	for _, claimTemplate := range p.NodeSpec.VolumeClaimTemplates {
		persistentVolumes = append(persistentVolumes, corev1.Volume{
			Name: claimTemplate.Name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					// actual claim name will be resolved and fixed right before pod creation
					ClaimName: "claim-name-placeholder",
				},
			},
		})
	}

	// build on top of the user-provided pod template spec
	builder := defaults.NewPodTemplateBuilder(p.NodeSpec.PodTemplate, v1alpha1.ElasticsearchContainerName).
		WithDockerImage(p.Elasticsearch.Spec.Image, stringsutil.Concat(pod.DefaultImageRepository, ":", p.Elasticsearch.Spec.Version)).
		WithResources(DefaultResources).
		WithTerminationGracePeriod(pod.DefaultTerminationGracePeriodSeconds).
		WithPorts(pod.DefaultContainerPorts).
		WithReadinessProbe(*pod.NewReadinessProbe()).
		WithAffinity(pod.DefaultAffinity(p.Elasticsearch.Name)).
		WithEnv(newEnvironmentVarsFn(p)...)

	// setup init containers
	initContainers, err := newInitContainersFn(
		builder.Container.Image,
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
			append(
				persistentVolumes, // includes the data volume, unless specified differently in the pod template
				append(
					initcontainer.PluginVolumes.Volumes(),
					esvolume.DefaultLogsVolume,
					p.UsersSecretVolume.Volume(),
					p.UnicastHostsVolume.Volume(),
					probeSecret.Volume(),
					transportCertificatesVolume.Volume(),
					httpCertificatesVolume.Volume(),
					scriptsVolume.Volume(),
					configVolume.Volume(),
				)...)...).
		WithVolumeMounts(
			append(
				initcontainer.PluginVolumes.EsContainerVolumeMounts(),
				esvolume.DefaultDataVolumeMount,
				esvolume.DefaultLogsVolumeMount,
				p.UsersSecretVolume.VolumeMount(),
				p.UnicastHostsVolume.VolumeMount(),
				probeSecret.VolumeMount(),
				transportCertificatesVolume.VolumeMount(),
				httpCertificatesVolume.VolumeMount(),
				scriptsVolume.VolumeMount(),
				configVolume.VolumeMount(),
			)...)

	if p.KeystoreResources != nil {
		builder = builder.
			WithVolumes(p.KeystoreResources.Volume).
			WithInitContainers(p.KeystoreResources.InitContainer)
	}

	builder = builder.
		WithInitContainers(initContainers...).
		WithInitContainerDefaults()

	// set labels
	version, err := version.Parse(p.Elasticsearch.Spec.Version)
	if err != nil {
		return pod.PodSpecContext{}, err
	}
	unpackedCfg, err := cfg.Unpack()
	if err != nil {
		return pod.PodSpecContext{}, err
	}
	nodeRoles := unpackedCfg.Node
	// label with a hash of the config to rotate the pod on config changes
	cfgHash := hash.HashObject(cfg)
	podLabels, err := label.NewPodLabels(k8s.ExtractNamespacedName(&p.Elasticsearch), statefulSetName, *version, nodeRoles, cfgHash)
	if err != nil {
		return pod.PodSpecContext{}, err
	}
	if p.KeystoreResources != nil {
		// label with a checksum of the secure settings to rotate the pod on secure settings change
		// TODO: use hash.HashObject instead && fix the config checksum label name?
		configChecksum := sha256.New224()
		_, _ = configChecksum.Write([]byte(p.KeystoreResources.Version))
		podLabels[label.ConfigChecksumLabelName] = fmt.Sprintf("%x", configChecksum.Sum(nil))
	}
	builder = builder.WithLabels(podLabels)

	return pod.PodSpecContext{
		NodeSpec:    p.NodeSpec,
		PodTemplate: builder.PodTemplate,
	}, nil
}

// quantityToMegabytes returns the megabyte value of the provided resource.Quantity
func quantityToMegabytes(q resource.Quantity) int {
	return int(q.Value()) / 1024 / 1024
}
