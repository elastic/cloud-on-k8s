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
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/user"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
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
	es v1alpha1.Elasticsearch,
	paramsTmpl pod.NewPodSpecParams,
	newEnvironmentVarsFn func(pod.NewPodSpecParams, volume.SecretVolume, volume.SecretVolume) []corev1.EnvVar,
	newInitContainersFn func(imageName string, operatorImage string, setVMMaxMapCount bool, nodeCertificatesVolume volume.SecretVolume) ([]corev1.Container, error),
	operatorImage string,
) ([]pod.PodSpecContext, error) {
	podSpecs := make([]pod.PodSpecContext, 0, es.Spec.NodeCount())

	for _, topoElem := range es.Spec.Topology {
		for i := int32(0); i < topoElem.NodeCount; i++ {
			podSpec, err := podSpec(
				pod.NewPodSpecParams{
					Version:         es.Spec.Version,
					LicenseType:     es.Spec.GetLicenseType(),
					CustomImageName: es.Spec.Image,
					ClusterName:     es.Name,
					DiscoveryZenMinimumMasterNodes: settings.ComputeMinimumMasterNodes(
						es.Spec.Topology,
					),
					DiscoveryServiceName: services.DiscoveryServiceName(es.Name),
					NodeTypes:            topoElem.NodeTypes,
					Affinity:             topoElem.PodTemplate.Spec.Affinity,
					SetVMMaxMapCount:     es.Spec.SetVMMaxMapCount,
					Resources:            topoElem.Resources,
					UsersSecretVolume:    paramsTmpl.UsersSecretVolume,
					ConfigMapVolume:      paramsTmpl.ConfigMapVolume,
					ExtraFilesRef:        paramsTmpl.ExtraFilesRef,
					KeystoreSecretRef:    paramsTmpl.KeystoreSecretRef,
					ProbeUser:            paramsTmpl.ProbeUser,
					ReloadCredsUser:      paramsTmpl.ReloadCredsUser,
				},
				operatorImage,
				newEnvironmentVarsFn,
				newInitContainersFn,
			)
			if err != nil {
				return nil, err
			}

			podSpecs = append(podSpecs, pod.PodSpecContext{PodSpec: podSpec, TopologyElement: topoElem})
		}
	}

	return podSpecs, nil
}

// podSpec creates a new PodSpec for an Elasticsearch node
func podSpec(
	p pod.NewPodSpecParams,
	operatorImage string,
	newEnvironmentVarsFn func(pod.NewPodSpecParams, volume.SecretVolume, volume.SecretVolume) []corev1.EnvVar,
	newInitContainersFn func(elasticsearchImage string, operatorImage string, setVMMaxMapCount bool, nodeCertificatesVolume volume.SecretVolume) ([]corev1.Container, error),
) (corev1.PodSpec, error) {
	elasticsearchImage := stringsutil.Concat(pod.DefaultImageRepository, ":", p.Version)
	if p.CustomImageName != "" {
		elasticsearchImage = p.CustomImageName
	}

	terminationGracePeriodSeconds := pod.DefaultTerminationGracePeriodSeconds
	volumes := map[string]volume.VolumeLike{
		p.ConfigMapVolume.Name():   p.ConfigMapVolume,
		p.UsersSecretVolume.Name(): p.UsersSecretVolume,
	}

	probeSecret := volume.NewSelectiveSecretVolumeWithMountPath(
		user.ElasticInternalUsersSecretName(p.ClusterName), volume.ProbeUserVolumeName,
		volume.ProbeUserSecretMountPath, []string{p.ProbeUser.Name},
	)
	volumes[probeSecret.Name()] = probeSecret

	reloadCredsSecret := volume.NewSelectiveSecretVolumeWithMountPath(
		user.ElasticInternalUsersSecretName(p.ClusterName), volume.ReloadCredsUserVolumeName,
		volume.ReloadCredsUserSecretMountPath, []string{p.ReloadCredsUser.Name},
	)
	volumes[reloadCredsSecret.Name()] = reloadCredsSecret

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
			Image:           elasticsearchImage,
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
				initcontainer.PrepareFsSharedVolumes.EsContainerVolumeMounts(),
				initcontainer.PrivateKeySharedVolume.EsContainerVolumeMount(),
				p.UsersSecretVolume.VolumeMount(),
				p.ConfigMapVolume.VolumeMount(),
				probeSecret.VolumeMount(),
				extraFilesSecretVolume.VolumeMount(),
				nodeCertificatesVolume.VolumeMount(),
			),
			Command: []string{
				"/usr/share/elasticsearch/bin/process-manager",
				"--name", "es",
				"--cmd", "/usr/local/bin/docker-entrypoint.sh",
			},
		}},
		TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
		Volumes: append(
			initcontainer.PrepareFsSharedVolumes.Volumes(),
			initcontainer.PrivateKeySharedVolume.Volume(),
			p.UsersSecretVolume.Volume(),
			p.ConfigMapVolume.Volume(),
			probeSecret.Volume(),
			reloadCredsSecret.Volume(),
			extraFilesSecretVolume.Volume(),
			keystoreVolume.Volume(),
		),
		AutomountServiceAccountToken: &automountServiceAccountToken,
	}

	// Setup init containers
	initContainers, err := newInitContainersFn(elasticsearchImage, operatorImage, p.SetVMMaxMapCount, nodeCertificatesVolume)
	if err != nil {
		return corev1.PodSpec{}, err
	}
	podSpec.InitContainers = initContainers
	return podSpec, nil
}

// NewPod constructs a pod from the given parameters.
func NewPod(
	version version.Version,
	es v1alpha1.Elasticsearch,
	podSpecCtx pod.PodSpecContext,
) (corev1.Pod, error) {
	labels := label.NewLabels(k8s.ExtractNamespacedName(&es))
	// add labels from the version
	labels[label.VersionLabelName] = version.String()

	// add labels for node types
	label.NodeTypesMasterLabelName.Set(podSpecCtx.TopologyElement.NodeTypes.Master, labels)
	label.NodeTypesDataLabelName.Set(podSpecCtx.TopologyElement.NodeTypes.Data, labels)
	label.NodeTypesIngestLabelName.Set(podSpecCtx.TopologyElement.NodeTypes.Ingest, labels)
	label.NodeTypesMLLabelName.Set(podSpecCtx.TopologyElement.NodeTypes.ML, labels)

	// add user-defined labels, unless we already manage a label matching the same key. we might want to consider
	// issuing at least a warning in this case due to the potential for unexpected behavior
	for k, v := range podSpecCtx.TopologyElement.PodTemplate.Labels {
		if _, ok := labels[k]; !ok {
			labels[k] = v
		}
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pod.NewNodeName(es.Name),
			Namespace:   es.Namespace,
			Labels:      labels,
			Annotations: podSpecCtx.TopologyElement.PodTemplate.Annotations,
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
