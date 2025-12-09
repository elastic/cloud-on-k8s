// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"crypto/sha256"
	"fmt"
	"hash/fnv"
	"path"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	common_deployment "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	common_name "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
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
	// according to the Policy name, and associated Elasticsearch name.
	AutoOpsNamer = common_name.NewNamer("autoops")
	// Default resources for the AutoOps Agent deployment.
	// These currently mirror the defaults for the Elastic Agent deployment.
	defaultResources = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("400Mi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("400Mi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
	}
)

func (r *ReconcileAutoOpsAgentPolicy) deploymentParams(ctx context.Context, policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) (appsv1.Deployment, error) {
	v, err := version.Parse(policy.Spec.Version)
	if err != nil {
		return appsv1.Deployment{}, err
	}
	labels := map[string]string{
		commonv1.TypeLabelName: "autoops-agent",
		autoOpsLabelName:       policy.Name,
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
	podTemplateSpec := defaults.NewPodTemplateBuilder(policy.Spec.PodTemplate, "autoops-agent").
		WithArgs("--config", path.Join(configVolumePath, autoOpsESConfigFileName)).
		WithLabels(meta.Labels).
		WithAnnotations(meta.Annotations).
		WithDockerImage(policy.Spec.Image, container.ImageRepository(container.AutoOpsAgentImage, v)).
		WithEnv(autoopsEnvVars(es)...).
		WithResources(defaultResources).
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

	// Hash ES namespace and name to create a short unique identifier
	// preventing name length issues.
	esIdentifier := es.GetNamespace() + es.GetName()
	esHash := fmt.Sprintf("%x", sha256.Sum256([]byte(esIdentifier)))[0:6]

	return common_deployment.New(common_deployment.Params{
		Name:      AutoOpsNamer.Suffix(policy.GetName(), esHash),
		Namespace: policy.GetNamespace(),
		Selector: map[string]string{
			autoOpsLabelName: policy.GetName(),
		},
		Metadata:             meta,
		PodTemplateSpec:      podTemplateSpec,
		Replicas:             1,
		RevisionHistoryLimit: policy.Spec.RevisionHistoryLimit,
	}), nil
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

// buildConfigHash builds a hash of the ConfigMap data and secret values
// to trigger pod restart on config changes
func buildConfigHash(ctx context.Context, c k8s.Client, policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) (string, error) {
	configHash := fnv.New32a()

	// Hash ConfigMap data
	configMapName := fmt.Sprintf("%s-%s-%s", autoOpsESConfigMapName, es.Namespace, es.Name)
	var configMap corev1.ConfigMap
	configMapKey := types.NamespacedName{Namespace: policy.Namespace, Name: configMapName}
	if err := c.Get(ctx, configMapKey, &configMap); err != nil {
		return "", err
	}

	if configData, ok := configMap.Data[autoOpsESConfigFileName]; ok {
		_, _ = configHash.Write([]byte(configData))
	}

	// Hash secret values from autoops-secret
	autoopsSecretKey := types.NamespacedName{Namespace: policy.Namespace, Name: "autoops-secret"}
	var autoopsSecret corev1.Secret
	if err := c.Get(ctx, autoopsSecretKey, &autoopsSecret); err != nil {
		return "", fmt.Errorf("failed to get autoops-secret: %w", err)
	}

	// Hash secret keys, including optional keys. There's no code here to handle missing keys as
	// 1. The optional keys are included here.
	// 2. The required keys are already validated in the controller, so they should always be present.
	requiredKeys := []string{"autoops-token", "autoops-otel-url", "cloud-connected-mode-api-key", "cloud-connected-mode-api-url"}
	for _, key := range requiredKeys {
		if data, ok := autoopsSecret.Data[key]; ok {
			_, _ = configHash.Write(data)
		}
	}

	// Hash ES API key secret
	esAPIKeySecretName := apiKeySecretNameFor(types.NamespacedName{Namespace: es.Namespace, Name: es.Name})
	esAPIKeySecretKey := types.NamespacedName{Namespace: policy.Namespace, Name: esAPIKeySecretName}
	var esAPIKeySecret corev1.Secret
	if err := c.Get(ctx, esAPIKeySecretKey, &esAPIKeySecret); err != nil {
		return "", fmt.Errorf("failed to get ES API key secret %s: %w", esAPIKeySecretName, err)
	}

	// This data may not exist on initial reconciliation, so we don't return an error if it's missing.
	// This should resolve itself on the next reconciliation after the API key is created.
	if apiKeyData, ok := esAPIKeySecret.Data["api_key"]; ok {
		_, _ = configHash.Write(apiKeyData)
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
						Name: apiKeySecretNameFor(types.NamespacedName{Namespace: es.Namespace, Name: es.Name}),
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
