// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"fmt"
	"hash/fnv"
	"path"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/pointer"
)

const (
	autoOpsLabelName         = "autoops.k8s.elastic.co/name"
	configVolumeName         = "config-volume"
	configVolumePath         = "/mnt/config"
	configHashAnnotationName = "autoops.k8s.elastic.co/config-hash"
	readinessProbePort       = 13133
)

var (
	// ESNAutoOpsNamer is a Namer that generates names for AutoOps deployments
	// according to the Policy name, and associated Elasticsearch name ensuring
	// the name doesn't exceed the max length of 27 characters for deployments.
	AutoOpsNamer    = common_name.NewNamer("autoops").WithMaxNameLength(27)
	basePodTemplate = corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "autoops-agent",
				},
			},
		},
	}
)

type ExpectedResources struct {
	deployment appsv1.Deployment
}

func (r *ReconcileAutoOpsAgentPolicy) generateExpectedResources(ctx context.Context, policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) (ExpectedResources, error) {
	deployment, err := r.deploymentParams(ctx, policy, es)
	if err != nil {
		return ExpectedResources{}, err
	}
	return ExpectedResources{
		deployment: deployment,
	}, nil
}

func (r *ReconcileAutoOpsAgentPolicy) deploymentParams(ctx context.Context, policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) (appsv1.Deployment, error) {
	var deployment appsv1.Deployment
	v, err := version.Parse(policy.Spec.Version)
	if err != nil {
		return appsv1.Deployment{}, err
	}
	labels := map[string]string{
		commonv1.TypeLabelName: "autoops-agent",
		autoOpsLabelName:       policy.Name,
	}
	deployment.ObjectMeta = metav1.ObjectMeta{
		Name:      AutoOpsNamer.Suffix(policy.GetName(), es.GetName(), es.GetNamespace()),
		Namespace: policy.GetNamespace(),
		Labels:    labels,
	}
	deployment.Spec = appsv1.DeploymentSpec{
		Replicas: pointer.Int32(1),
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				autoOpsLabelName: policy.GetName(),
			},
		},
	}
	// Create ES-specific config map volume
	configMapName := fmt.Sprintf("%s-%s-%s", autoOpsESConfigMapName, es.Namespace, es.Name)
	configVolume := volume.NewConfigMapVolume(configMapName, configVolumeName, configVolumePath)

	volumes := []corev1.Volume{configVolume.Volume()}
	volumeMounts := []corev1.VolumeMount{configVolume.VolumeMount()}

	// Add CA certificate volume for this ES instance only if TLS is enabled
	if es.Spec.HTTP.TLS.Enabled() {
		caSecretName := fmt.Sprintf("%s-%s-%s", es.Name, es.Namespace, autoOpsESCASecretSuffix)
		caVolume := volume.NewSecretVolumeWithMountPath(
			caSecretName,
			fmt.Sprintf("es-ca-%s-%s", es.Name, es.Namespace),
			fmt.Sprintf("/mnt/elastic-internal/es-ca/%s-%s", es.Namespace, es.Name),
		)
		volumes = append(volumes, caVolume.Volume())
		volumeMounts = append(volumeMounts, caVolume.VolumeMount())
	}

	// Build config hash from ConfigMap to trigger pod restart on config changes
	configHash, err := buildConfigHash(ctx, r.Client, policy, es)
	if err != nil {
		return appsv1.Deployment{}, err
	}

	annotations := map[string]string{configHashAnnotationName: configHash}
	meta := metadata.Propagate(&policy, metadata.Metadata{Labels: labels, Annotations: annotations})
	podTemplateSpec := defaults.NewPodTemplateBuilder(basePodTemplate, "autoops-agent").
		WithArgs("--config", path.Join(configVolumePath, autoOpsESConfigFileName)).
		WithLabels(meta.Labels).
		WithAnnotations(meta.Annotations).
		WithDockerImage(container.ImageRepository(container.AutoOpsAgentImage, v), v.String()).
		WithEnv(autoopsEnvVars(es)...).
		WithVolumes(volumes...).
		WithVolumeMounts(volumeMounts...).
		WithPorts([]corev1.ContainerPort{{Name: "http", ContainerPort: int32(readinessProbePort), Protocol: corev1.ProtocolTCP}}).
		WithReadinessProbe(readinessProbe()).
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

// readinessProbe is the readiness probe for the AutoOps Agent container
func readinessProbe() corev1.Probe {
	scheme := corev1.URISchemeHTTP
	return corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      5,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(readinessProbePort),
				Path:   "/health/status",
				Scheme: scheme,
			},
		},
	}
}

// buildConfigHash builds a hash of the ConfigMap data to trigger pod restart on config changes
func buildConfigHash(ctx context.Context, c k8s.Client, policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) (string, error) {
	configHash := fnv.New32a()

	configMapName := fmt.Sprintf("%s-%s-%s", autoOpsESConfigMapName, es.Namespace, es.Name)
	var configMap corev1.ConfigMap
	configMapKey := types.NamespacedName{Namespace: policy.Namespace, Name: configMapName}
	if err := c.Get(ctx, configMapKey, &configMap); err != nil {
		return "", err
	}

	if configData, ok := configMap.Data[autoOpsESConfigFileName]; ok {
		_, _ = configHash.Write([]byte(configData))
	}

	return fmt.Sprint(configHash.Sum32()), nil
}

// autoopsEnvVars returns the environment variables for the AutoOps deployment
// that reference values from the autoops-secret and the ES elastic user secret.
func autoopsEnvVars(es esv1.Elasticsearch) []corev1.EnvVar {
	esService := services.InternalServiceURL(es)
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
			Name:  "AUTOOPS_ES_URL",
			Value: esService,
		},
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
			Name: "AUTOOPS_ES_API_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: apiKeySecretNameFrom(es),
					},
					Key:      "api_key",
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
