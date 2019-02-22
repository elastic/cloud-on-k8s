// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version

import (
	"context"
	"fmt"
	"path"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/secret"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	defaultMemoryLimits = resource.MustParse("1Gi")
	SecurityPropsFile   = path.Join(settings.ManagedConfigPath, settings.SecurityPropsFile)
)

// NewExpectedPodSpecs creates PodSpecContexts for all Elasticsearch nodes in the given Elasticsearch cluster
func NewExpectedPodSpecs(
	es v1alpha1.ElasticsearchCluster,
	paramsTmpl pod.NewPodSpecParams,
	newEnvironmentVarsFn func(pod.NewPodSpecParams, volume.SecretVolume, volume.SecretVolume) []corev1.EnvVar,
	newInitContainersFn func(imageName string, operatorImage string, setVMMaxMapCount bool) ([]corev1.Container, error),
	newSideCarContainersFn func(imageName string, spec pod.NewPodSpecParams, volumes map[string]volume.VolumeLike) ([]corev1.Container, error),
	additionalVolumes []corev1.Volume,
	operatorImage string,
) ([]pod.PodSpecContext, error) {
	podSpecs := make([]pod.PodSpecContext, 0, es.Spec.NodeCount())

	for _, topology := range es.Spec.Topologies {
		for i := int32(0); i < topology.NodeCount; i++ {
			podSpec, err := podSpec(
				pod.NewPodSpecParams{
					Version:         es.Spec.Version,
					CustomImageName: es.Spec.Image,
					ClusterName:     es.Name,
					DiscoveryZenMinimumMasterNodes: settings.ComputeMinimumMasterNodes(
						es.Spec.Topologies,
					),
					DiscoveryServiceName: services.DiscoveryServiceName(es.Name),
					NodeTypes:            topology.NodeTypes,
					Affinity:             topology.PodTemplate.Spec.Affinity,
					SetVMMaxMapCount:     es.Spec.SetVMMaxMapCount,
					Resources:            topology.Resources,
					UsersSecretVolume:    paramsTmpl.UsersSecretVolume,
					ConfigMapVolume:      paramsTmpl.ConfigMapVolume,
					ExtraFilesRef:        paramsTmpl.ExtraFilesRef,
					KeystoreSecretRef:    paramsTmpl.KeystoreSecretRef,
					ProbeUser:            paramsTmpl.ProbeUser,
				},
				operatorImage,
				newEnvironmentVarsFn,
				newInitContainersFn,
				newSideCarContainersFn,
				additionalVolumes,
			)
			if err != nil {
				return nil, err
			}

			podSpecs = append(podSpecs, pod.PodSpecContext{PodSpec: podSpec, TopologySpec: topology})
		}
	}

	return podSpecs, nil
}

// podSpec creates a new PodSpec for an Elasticsearch node
func podSpec(
	p pod.NewPodSpecParams,
	operatorImage string,
	newEnvironmentVarsFn func(pod.NewPodSpecParams, volume.SecretVolume, volume.SecretVolume) []corev1.EnvVar,
	newInitContainersFn func(imageName string, operatorImage string, setVMMaxMapCount bool) ([]corev1.Container, error),
	newSideCarContainersFn func(imageName string, spec pod.NewPodSpecParams, volumes map[string]volume.VolumeLike) ([]corev1.Container, error),
	additionalVolumes []corev1.Volume,
) (corev1.PodSpec, error) {
	imageName := stringsutil.Concat(pod.DefaultImageRepository, ":", p.Version)
	if p.CustomImageName != "" {
		imageName = p.CustomImageName
	}

	terminationGracePeriodSeconds := pod.DefaultTerminationGracePeriodSeconds
	volumes := map[string]volume.VolumeLike{
		p.ConfigMapVolume.Name():   p.ConfigMapVolume,
		p.UsersSecretVolume.Name(): p.UsersSecretVolume,
	}

	probeSecret := volume.NewSelectiveSecretVolumeWithMountPath(
		secret.ElasticInternalUsersSecretName(p.ClusterName), volume.ProbeUserVolumeName,
		volume.ProbeUserSecretMountPath, []string{p.ProbeUser.Name},
	)
	volumes[probeSecret.Name()] = probeSecret

	extraFilesSecretVolume := volume.NewSecretVolumeWithMountPath(
		p.ExtraFilesRef.Name,
		"extrafiles",
		"/usr/share/elasticsearch/config/extrafiles",
	)

	volumes[extraFilesSecretVolume.Name()] = extraFilesSecretVolume

	// we don't have a secret name for this, this will be injected as a volume for us upon creation, this is fine
	// because we will not be adding this to the container Volumes, only the VolumeMounts section.
	nodeCertificatesVolume := volume.NewSecretVolumeWithMountPath(
		"",
		volume.NodeCertificatesSecretVolumeName,
		volume.NodeCertificatesSecretVolumeMountPath,
	)

	volumes[nodeCertificatesVolume.Name()] = nodeCertificatesVolume

	keystoreVolume := volume.NewSecretVolumeWithMountPath(
		p.KeystoreSecretRef.Name,
		keystore.SecretVolumeName,
		keystore.SecretMountPath)

	volumes[keystoreVolume.Name()] = keystoreVolume

	resourceLimits := corev1.ResourceList{
		corev1.ResourceMemory: nonZeroQuantityOrDefault(*p.Resources.Limits.Memory(), defaultMemoryLimits),
	}
	if !p.Resources.Limits.Cpu().IsZero() {
		resourceLimits[corev1.ResourceCPU] = *p.Resources.Limits.Cpu()
	}

	// TODO: Security Context
	automountServiceAccountToken := false
	podSpec := corev1.PodSpec{
		Affinity: p.Affinity,

		Containers: []corev1.Container{{
			Env:             newEnvironmentVarsFn(p, nodeCertificatesVolume, extraFilesSecretVolume),
			Image:           imageName,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Name:            pod.DefaultContainerName,
			Ports:           pod.DefaultContainerPorts,
			Resources: corev1.ResourceRequirements{
				Limits: resourceLimits,
				// we do not specify Requests here in order to end up in the qosClass of Guaranteed.
				// see https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/ for more details
			},
			ReadinessProbe: &corev1.Probe{
				FailureThreshold:    3,
				InitialDelaySeconds: 10,
				PeriodSeconds:       10,
				SuccessThreshold:    3,
				TimeoutSeconds:      5,
				Handler: corev1.Handler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"sh",
							"-c",
							pod.DefaultReadinessProbeScript,
						},
					},
				},
			},
			VolumeMounts: append(
				initcontainer.SharedVolumes.EsContainerVolumeMounts(),
				p.UsersSecretVolume.VolumeMount(),
				p.ConfigMapVolume.VolumeMount(),
				probeSecret.VolumeMount(),
				extraFilesSecretVolume.VolumeMount(),
				nodeCertificatesVolume.VolumeMount(),
			),
		}},
		TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
		Volumes: append(
			initcontainer.SharedVolumes.Volumes(),
			p.UsersSecretVolume.Volume(),
			p.ConfigMapVolume.Volume(),
			probeSecret.Volume(),
			extraFilesSecretVolume.Volume(),
			keystoreVolume.Volume(),
		),
		AutomountServiceAccountToken: &automountServiceAccountToken,
	}

	podSpec.Volumes = append(podSpec.Volumes, additionalVolumes...)

	// Setup sidecars if any
	sidecars, err := newSideCarContainersFn(imageName, p, volumes)
	if err != nil {
		return corev1.PodSpec{}, err
	}
	podSpec.Containers = append(podSpec.Containers, sidecars...)

	// Setup init containers
	initContainers, err := newInitContainersFn(imageName, operatorImage, p.SetVMMaxMapCount)
	if err != nil {
		return corev1.PodSpec{}, err
	}
	podSpec.InitContainers = initContainers
	return podSpec, nil
}

// NewPod constructs a pod from the given parameters.
func NewPod(
	version version.Version,
	es v1alpha1.ElasticsearchCluster,
	podSpecCtx pod.PodSpecContext,
) (corev1.Pod, error) {
	labels := label.NewLabels(es)
	// add labels from the version
	labels[ElasticsearchVersionLabelName] = version.String()

	// add labels for node types
	label.NodeTypesMasterLabelName.Set(podSpecCtx.TopologySpec.NodeTypes.Master, labels)
	label.NodeTypesDataLabelName.Set(podSpecCtx.TopologySpec.NodeTypes.Data, labels)
	label.NodeTypesIngestLabelName.Set(podSpecCtx.TopologySpec.NodeTypes.Ingest, labels)
	label.NodeTypesMLLabelName.Set(podSpecCtx.TopologySpec.NodeTypes.ML, labels)

	// add user-defined labels, unless we already manage a label matching the same key. we might want to consider
	// issuing at least a warning in this case due to the potential for unexpected behavior
	for k, v := range podSpecCtx.TopologySpec.PodTemplate.Labels {
		if _, ok := labels[k]; !ok {
			labels[k] = v
		}
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pod.NewNodeName(es.Name),
			Namespace:   es.Namespace,
			Labels:      labels,
			Annotations: podSpecCtx.TopologySpec.PodTemplate.Annotations,
		},
		Spec: podSpecCtx.PodSpec,
	}

	return pod, nil
}

func UpdateZen1Discovery(esClient *client.Client, allPods []corev1.Pod) error {
	minimumMasterNodes := settings.ComputeMinimumMasterNodesFromPods(allPods)
	log.Info(fmt.Sprintf("Setting minimum master nodes to %d ", minimumMasterNodes))
	return esClient.SetMinimumMasterNodes(context.TODO(), minimumMasterNodes)
}

// MemoryLimitsToHeapSize converts a memory limit to the heap size (in megabytes) for the JVM
func MemoryLimitsToHeapSize(memoryLimit resource.Quantity) int {
	// use half the available memory as heap
	return quantityToMegabytes(nonZeroQuantityOrDefault(memoryLimit, defaultMemoryLimits)) / 2
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
