package version

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch/initcontainer"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newExpectedPodSpecs creates PodSpecContexts for all Elasticsearch nodes in the given stack
func newExpectedPodSpecs(
	stack v1alpha1.Stack,
	paramsTmpl elasticsearch.NewPodSpecParams,
	newEnvironmentVarsFn func(elasticsearch.NewPodSpecParams, elasticsearch.SecretVolume) []corev1.EnvVar,
	newInitContainersFn func(imageName string, ki initcontainer.KeystoreInit, setVMMaxMapCount bool) ([]corev1.Container, error),
) ([]elasticsearch.PodSpecContext, error) {
	podSpecs := make([]elasticsearch.PodSpecContext, 0, stack.Spec.Elasticsearch.NodeCount())

	for _, topology := range stack.Spec.Elasticsearch.Topologies {
		for i := int32(0); i < topology.NodeCount; i++ {
			podSpec, err := newPodSpec(elasticsearch.NewPodSpecParams{
				Version:         stack.Spec.Version,
				CustomImageName: stack.Spec.Elasticsearch.Image,
				ClusterName:     stack.Name,
				DiscoveryZenMinimumMasterNodes: elasticsearch.ComputeMinimumMasterNodes(
					stack.Spec.Elasticsearch.Topologies,
				),
				DiscoveryServiceName: elasticsearch.DiscoveryServiceName(stack.Name),
				NodeTypes:            topology.NodeTypes,
				Affinity:             topology.PodTemplate.Spec.Affinity,
				SetVMMaxMapCount:     stack.Spec.Elasticsearch.SetVMMaxMapCount,
				UsersSecretVolume:    paramsTmpl.UsersSecretVolume,
				ExtraFilesRef:        paramsTmpl.ExtraFilesRef,
				KeystoreConfig:       paramsTmpl.KeystoreConfig,
				ProbeUser:            paramsTmpl.ProbeUser,
			}, newEnvironmentVarsFn, newInitContainersFn)
			if err != nil {
				return nil, err
			}

			podSpecs = append(podSpecs, elasticsearch.PodSpecContext{PodSpec: podSpec, TopologySpec: topology})
		}
	}

	return podSpecs, nil
}

// newPodSpec creates a new PodSpec for an Elasticsearch node
func newPodSpec(
	p elasticsearch.NewPodSpecParams,
	newEnvironmentVarsFn func(elasticsearch.NewPodSpecParams, elasticsearch.SecretVolume) []corev1.EnvVar,
	newInitContainersFn func(imageName string, ki initcontainer.KeystoreInit, setVMMaxMapCount bool) ([]corev1.Container, error),
) (corev1.PodSpec, error) {
	imageName := common.Concat(elasticsearch.DefaultImageRepository, ":", p.Version)
	if p.CustomImageName != "" {
		imageName = p.CustomImageName
	}

	terminationGracePeriodSeconds := elasticsearch.DefaultTerminationGracePeriodSeconds

	probeSecret := elasticsearch.NewSelectiveSecretVolumeWithMountPath(
		elasticsearch.ElasticInternalUsersSecretName(p.ClusterName), "probe-user",
		elasticsearch.ProbeUserSecretMountPath, []string{p.ProbeUser.Name},
	)

	extraFilesSecretVolume := elasticsearch.NewSecretVolumeWithMountPath(
		p.ExtraFilesRef.Name,
		"extrafiles",
		"/usr/share/elasticsearch/config/extrafiles",
	)

	// TODO: Security Context
	podSpec := corev1.PodSpec{
		Affinity: p.Affinity,
		Containers: []corev1.Container{{
			Env:             newEnvironmentVarsFn(p, extraFilesSecretVolume),
			Image:           imageName,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Name:            elasticsearch.DefaultContainerName,
			Ports:           elasticsearch.DefaultContainerPorts,
			// TODO: Hardcoded resource limits and requests
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("800m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
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
							elasticsearch.DefaultReadinessProbeScript,
						},
					},
				},
			},
			VolumeMounts: append(
				initcontainer.SharedVolumes.EsContainerVolumeMounts(),
				p.UsersSecretVolume.VolumeMount(),
				probeSecret.VolumeMount(),
				extraFilesSecretVolume.VolumeMount(),
			),
		}},
		TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
		Volumes: append(
			initcontainer.SharedVolumes.Volumes(),
			p.UsersSecretVolume.Volume(),
			probeSecret.Volume(),
			extraFilesSecretVolume.Volume(),
		),
	}

	// keystore init is optional, will only happen if snapshots are requested in the stack resource
	keyStoreInit := initcontainer.KeystoreInit{Settings: p.KeystoreConfig.KeystoreSettings}
	if !p.KeystoreConfig.IsEmpty() {
		keystoreVolume := elasticsearch.NewSecretVolumeWithMountPath(
			p.KeystoreConfig.KeystoreSecretRef.Name,
			"keystore-init",
			elasticsearch.KeystoreSecretMountPath)

		podSpec.Volumes = append(podSpec.Volumes, keystoreVolume.Volume())
		keyStoreInit.VolumeMount = keystoreVolume.VolumeMount()
	}

	// Setup init containers
	initContainers, err := newInitContainersFn(
		imageName, keyStoreInit, p.SetVMMaxMapCount,
	)
	if err != nil {
		return corev1.PodSpec{}, err
	}
	podSpec.InitContainers = initContainers
	return podSpec, nil
}

// newPod constructs a pod from the given parameters.
func newPod(
	versionStrategy ElasticsearchVersionStrategy,
	stack v1alpha1.Stack,
	podSpecCtx elasticsearch.PodSpecContext,
) (corev1.Pod, error) {
	labels := elasticsearch.NewLabels(stack)

	// add labels from the version strategy
	for k, v := range versionStrategy.NewPodLabels() {
		labels[k] = v
	}

	// add user-defined labels, unless we already manage a label matching the same key. we might want to consider
	// issuing at least a warning in this case due to the potential for unexpected behavior
	for k, v := range podSpecCtx.TopologySpec.PodTemplate.Labels {
		if _, ok := labels[k]; !ok {
			labels[k] = v
		}
	}

	pod := corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        elasticsearch.NewNodeName(stack.Name),
			Namespace:   stack.Namespace,
			Labels:      labels,
			Annotations: podSpecCtx.TopologySpec.PodTemplate.Annotations,
		},
		Spec: podSpecCtx.PodSpec,
	}

	return pod, nil
}
