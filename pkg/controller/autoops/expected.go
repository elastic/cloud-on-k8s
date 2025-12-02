package autoops

import (
	"path"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	common_name "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/pointer"
)

const (
	configVolumeName = "config-volume"
	configVolumePath = "/mnt/config"
)

var (
	// ESNAutoOpsNamer is a Namer that generates names for AutoOps deployments
	// according to the associated Elasticsearch cluster name.
	AutoOpsNamer    = common_name.NewNamer("autoops")
	basePodTemplate = corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "autoops-agent",
				},
			},
		},
	}
	configVolume = volume.NewConfigMapVolume(autoOpsESConfigMapName, configVolumeName, configVolumePath)
)

type ExpectedResources struct {
	deployment appsv1.Deployment
}

func (r *ReconcileAutoOpsAgentPolicy) generateExpectedResources(policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) (ExpectedResources, error) {
	deployment, err := r.deploymentParams(policy, es)
	if err != nil {
		return ExpectedResources{}, err
	}
	return ExpectedResources{
		deployment: deployment,
	}, nil
}

func (r *ReconcileAutoOpsAgentPolicy) deploymentParams(policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) (appsv1.Deployment, error) {
	var deployment appsv1.Deployment
	v, err := version.Parse(policy.Spec.Version)
	if err != nil {
		return appsv1.Deployment{}, err
	}
	labels := map[string]string{
		commonv1.TypeLabelName:        "autoops-agent",
		"autoops.k8s.elastic.co/name": policy.Name,
	}
	deployment.ObjectMeta = metav1.ObjectMeta{
		Name:      AutoOpsNamer.Suffix(es.Name, es.GetNamespace(), "deploy"),
		Namespace: policy.GetNamespace(),
		Labels:    labels,
	}
	deployment.Spec = appsv1.DeploymentSpec{
		Replicas: pointer.Int32(1),
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"autoops.k8s.elastic.co/name": policy.Name,
			},
		},
	}
	volumes := []corev1.Volume{configVolume.Volume()}
	volumeMounts := []corev1.VolumeMount{configVolume.VolumeMount()}
	meta := metadata.Propagate(&policy, metadata.Metadata{Labels: labels, Annotations: nil})
	podTemplateSpec := defaults.NewPodTemplateBuilder(basePodTemplate, "autoops-agent").
		WithArgs("--config", path.Join(configVolumePath, autoOpsESConfigFileName)).
		WithLabels(meta.Labels).
		WithAnnotations(meta.Annotations).
		WithDockerImage(container.ImageRepository(container.AutoOpsAgentImage, v), v.String()).
		WithEnv(autoopsEnvVars(types.NamespacedName{Namespace: es.Namespace, Name: es.Name})...).
		WithVolumes(volumes...).
		WithVolumeMounts(volumeMounts...).
		WithContainersSecurityContext(corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			Privileged: ptr.To(false),
			// Can't set this to true because of:
			// failed to build pipelines:
			// failed to create "metricbeatreceiver" receiver for data type "logs":
			// error creating metricbeatreceiver: error loading meta data:
			// failed to create Beat meta file: open data/meta.json.new: read-only file system
			ReadOnlyRootFilesystem: ptr.To(false),
			// Can't currently do this because of:
			// Error: container has runAsNonRoot and image has non-numeric user (elastic-agent)
			// RunAsNonRoot:           ptr.To(true),
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		}).
		PodTemplate

	deployment.Spec.Template = podTemplateSpec
	return deployment, nil
}

// autoopsEnvVars returns the environment variables for the AutoOps deployment
// that reference values from the autoops-secret and the ES elastic user secret.
func autoopsEnvVars(esNN types.NamespacedName) []corev1.EnvVar {
	// Encode the secret key using base64 URL encoding to ensure it matches the regex '[-._a-zA-Z0-9]+'
	secretKey := encodeESSecretKey(esNN.Namespace, esNN.Name)
	return []corev1.EnvVar{
		{
			Name: "AUTOOPS_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "autoops-secret",
					},
					Key: "autoops-token",
				},
			},
		},
		{
			Name: "AUTOOPS_ES_URL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "autoops-secret",
					},
					Key: "autoops-es-url",
				},
			},
		},
		// {
		// 	Name: "AUTOOPS_TEMP_RESOURCE_ID",
		// 	ValueFrom: &corev1.EnvVarSource{
		// 		SecretKeyRef: &corev1.SecretKeySelector{
		// 			LocalObjectReference: corev1.LocalObjectReference{
		// 				Name: "autoops-secret",
		// 			},
		// 			Key: "temp-resource-id",
		// 		},
		// 	},
		// },
		{
			Name: "AUTOOPS_OTEL_URL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "autoops-secret",
					},
					Key: "autoops-otel-url",
				},
			},
		},
		{
			Name:  "AUTOOPS_ES_USERNAME",
			Value: "elastic-internal-monitoring",
		},
		{
			Name: "AUTOOPS_ES_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: autoOpsESPasswordsSecretName,
					},
					Key:      secretKey,
					Optional: ptr.To(false),
				},
			},
		},
		{
			Name: "ELASTIC_CLOUD_CONNECTED_MODE_API_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "autoops-secret",
					},
					Key: "cloud-connected-mode-api-key",
				},
			},
		},
		{
			Name: "ELASTIC_CLOUD_CONNECTED_MODE_API_URL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "autoops-secret",
					},
					Key:      "cloud-connected-mode-api-url",
					Optional: ptr.To(true),
				},
			},
		},
	}
}
