// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

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

//
//// NewExpectedPodSpecs creates PodSpecContexts for all Elasticsearch nodes in the given Elasticsearch cluster
//func NewExpectedPodSpecs(
//	es v1alpha1.Elasticsearch,
//	paramsTmpl pod.NewPodSpecParams,
//	newEnvironmentVarsFn func(p pod.NewPodSpecParams, certs, creds, securecommon volume.SecretVolume) []corev1.EnvVar,
//	newESConfigFn func(clusterName string, config settings.CanonicalConfig) (settings.CanonicalConfig, error),
//	newInitContainersFn func(imageName string, operatorImage string, setVMMaxMapCount *bool, transportCerts volume.SecretVolume, clusterName string) ([]corev1.Container, error),
//	operatorImage string,
//) ([]pod.PodSpecContext, error) {
//	podSpecs := make([]pod.PodSpecContext, 0, es.Spec.NodeCount())
//
//	for _, node := range es.Spec.Nodes {
//		// add default PVCs to the node spec
//		node.VolumeClaimTemplates = defaults.AppendDefaultPVCs(
//			node.VolumeClaimTemplates, node.PodTemplate.Spec, esvolume.DefaultVolumeClaimTemplates...,
//		)
//
//		for i := int32(0); i < node.NodeCount; i++ {
//			params := pod.NewPodSpecParams{
//				// cluster-wide params
//				Elasticsearch: es,
//				// volumes
//				UsersSecretVolume:  paramsTmpl.UsersSecretVolume,
//				ProbeUser:          paramsTmpl.ProbeUser,
//				KeystoreUser:       paramsTmpl.KeystoreUser,
//				UnicastHostsVolume: paramsTmpl.UnicastHostsVolume,
//				// pod params
//				NodeSpec: node,
//			}
//			podSpecCtx, err := podSpecContext(
//				params,
//				operatorImage,
//				config,
//				newEnvironmentVarsFn,
//				newESConfigFn,
//				newInitContainersFn,
//			)
//			if err != nil {
//				return nil, err
//			}
//
//			podSpecs = append(podSpecs, podSpecCtx)
//		}
//	}
//
//	return podSpecs, nil
//}

// TODO: refactor
type PodTemplateSpecBuilder func(v1alpha1.NodeSpec, settings.CanonicalConfig) (corev1.PodTemplateSpec, error)

// TODO: refactor to avoid all the params mess
func BuildPodTemplateSpec(
	es v1alpha1.Elasticsearch,
	nodeSpec v1alpha1.NodeSpec,
	paramsTmpl pod.NewPodSpecParams,
	cfg settings.CanonicalConfig,
	newEnvironmentVarsFn func(p pod.NewPodSpecParams, certs, creds, securecommon volume.SecretVolume) []corev1.EnvVar,
	newInitContainersFn func(imageName string, operatorImage string, setVMMaxMapCount *bool, transportCerts volume.SecretVolume, clusterName string) ([]corev1.Container, error),
	operatorImage string,
) (corev1.PodTemplateSpec, error) {
	params := pod.NewPodSpecParams{
		// cluster-wide params
		Elasticsearch: es,
		// volumes
		UsersSecretVolume:  paramsTmpl.UsersSecretVolume,
		ProbeUser:          paramsTmpl.ProbeUser,
		KeystoreUser:       paramsTmpl.KeystoreUser,
		UnicastHostsVolume: paramsTmpl.UnicastHostsVolume,
		// pod params
		NodeSpec: nodeSpec,
	}
	podSpecCtx, err := podSpecContext(
		params,
		operatorImage,
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
	operatorImage string,
	cfg settings.CanonicalConfig,
	newEnvironmentVarsFn func(p pod.NewPodSpecParams, certs, creds, keystore volume.SecretVolume) []corev1.EnvVar,
	newInitContainersFn func(elasticsearchImage string, operatorImage string, setVMMaxMapCount *bool, transportCerts volume.SecretVolume, clusterName string) ([]corev1.Container, error),
) (pod.PodSpecContext, error) {
	statefulSetName := name.StatefulSet(p.Elasticsearch.Name, p.NodeSpec.Name)

	// setup volumes
	probeSecret := volume.NewSelectiveSecretVolumeWithMountPath(
		user.ElasticInternalUsersSecretName(p.Elasticsearch.Name), esvolume.ProbeUserVolumeName,
		esvolume.ProbeUserSecretMountPath, []string{p.ProbeUser.Name},
	)
	keystoreUserSecret := volume.NewSelectiveSecretVolumeWithMountPath(
		user.ElasticInternalUsersSecretName(p.Elasticsearch.Name), esvolume.KeystoreUserVolumeName,
		esvolume.KeystoreUserSecretMountPath, []string{p.KeystoreUser.Name},
	)
	secureSettingsVolume := volume.NewSecretVolumeWithMountPath(
		name.SecureSettingsSecret(p.Elasticsearch.Name),
		esvolume.SecureSettingsVolumeName,
		esvolume.SecureSettingsVolumeMountPath,
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
			append(
				persistentVolumes, // includes the data volume, unless specified differently in the pod template
				append(
					initcontainer.PluginVolumes.Volumes(),
					esvolume.DefaultLogsVolume,
					initcontainer.ProcessManagerVolume.Volume(),
					p.UsersSecretVolume.Volume(),
					p.UnicastHostsVolume.Volume(),
					probeSecret.Volume(),
					transportCertificatesVolume.Volume(),
					keystoreUserSecret.Volume(),
					secureSettingsVolume.Volume(),
					httpCertificatesVolume.Volume(),
					scriptsVolume.Volume(),
					configVolume.Volume(),
				)...)...).
		WithVolumeMounts(
			append(
				initcontainer.PluginVolumes.EsContainerVolumeMounts(),
				esvolume.DefaultDataVolumeMount,
				esvolume.DefaultLogsVolumeMount,
				initcontainer.ProcessManagerVolume.EsContainerVolumeMount(),
				p.UsersSecretVolume.VolumeMount(),
				p.UnicastHostsVolume.VolumeMount(),
				probeSecret.VolumeMount(),
				transportCertificatesVolume.VolumeMount(),
				keystoreUserSecret.VolumeMount(),
				secureSettingsVolume.VolumeMount(),
				httpCertificatesVolume.VolumeMount(),
				scriptsVolume.VolumeMount(),
				configVolume.VolumeMount(),
			)...).
		WithInitContainerDefaults().
		WithInitContainers(initContainers...)

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
	builder = builder.WithLabels(podLabels)

	return pod.PodSpecContext{
		NodeSpec:    p.NodeSpec,
		PodTemplate: builder.PodTemplate,
	}, nil
}

//
//// NewPod constructs a pod from the given parameters.
//func NewPod(
//	es v1alpha1.Elasticsearch,
//	podSpecCtx pod.PodSpecContext,
//) corev1.Pod {
//	// build a pod based on the podSpecCtx template
//	template := *podSpecCtx.PodTemplate.DeepCopy()
//	pod := corev1.Pod{
//		ObjectMeta: template.ObjectMeta,
//		Spec:       template.Spec,
//	}
//
//	// label the pod with a hash of its template, for comparison purpose,
//	// before it gets assigned a name
//	pod.Labels = hash.SetTemplateHashLabel(pod.Labels, template)
//
//	// set name & namespace
//	pod.Name = name.NewPodName(es.Name, podSpecCtx.NodeSpec)
//	pod.Namespace = es.Namespace
//
//	// set hostname and subdomain based on pod and cluster names
//	if pod.Spec.Hostname == "" {
//		pod.Spec.Hostname = pod.Name
//	}
//	if pod.Spec.Subdomain == "" {
//		pod.Spec.Subdomain = es.Name
//	}
//
//	return pod
//}

// quantityToMegabytes returns the megabyte value of the provided resource.Quantity
func quantityToMegabytes(q resource.Quantity) int {
	return int(q.Value()) / 1024 / 1024
}
