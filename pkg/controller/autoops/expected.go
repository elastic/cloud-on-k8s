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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonapikey "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/apikey"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	common_deployment "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	autoOpsLabelName         = "autoops.k8s.elastic.co/name"
	autoOpsAgentType         = "autoops-agent"
	configVolumeName         = "config-volume"
	configVolumePath         = "/mnt/config"
	configHashAnnotationName = "autoops.k8s.elastic.co/config-hash"
	readinessProbePort       = 13133
)

var (
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

// autoOpsConfigurationSecretNamespace returns the namespace for the AutoOps configuration secret.
// If the namespace is specified in AutoOpsRef use it, otherwise default to the policy namespace.
func autoOpsConfigurationSecretNamespace(policy autoopsv1alpha1.AutoOpsAgentPolicy) string {
	if policy.Spec.AutoOpsRef.Namespace != "" {
		return policy.Spec.AutoOpsRef.Namespace
	}
	return policy.GetNamespace()
}

// resourceLabelsFor returns the standard labels for AutoOps resources (Deployments, ConfigMaps, Secrets)
// associated with a specific policy and Elasticsearch cluster.
func resourceLabelsFor(policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) map[string]string {
	return map[string]string{
		commonv1.TypeLabelName:              autoOpsAgentType,
		autoOpsLabelName:                    policy.Name,
		commonapikey.MetadataKeyESName:      es.Name,
		commonapikey.MetadataKeyESNamespace: es.Namespace,
	}
}

func (r *AgentPolicyReconciler) deploymentParams(ctx context.Context, policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) (appsv1.Deployment, error) {
	v, err := version.Parse(policy.Spec.Version)
	if err != nil {
		return appsv1.Deployment{}, err
	}

	labels := resourceLabelsFor(policy, es)

	// Create ES-specific ConfigMap volume
	configMapName := autoopsv1alpha1.Config(policy.GetName(), es)
	configVolume := volume.NewConfigMapVolume(configMapName, configVolumeName, configVolumePath)

	volumes := []corev1.Volume{configVolume.Volume()}
	volumeMounts := []corev1.VolumeMount{configVolume.VolumeMount()}

	// Add CA certificate volume for this ES instance only if TLS is enabled
	if es.Spec.HTTP.TLS.Enabled() {
		caSecretName := autoopsv1alpha1.CASecret(policy.GetName(), es)
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
	podTemplateSpec := defaults.NewPodTemplateBuilder(policy.Spec.PodTemplate, autoOpsAgentType).
		WithArgs("--config", path.Join(configVolumePath, autoOpsESConfigFileName)).
		WithLabels(meta.Labels).
		WithAnnotations(meta.Annotations).
		WithDockerImage(policy.Spec.Image, container.ImageRepository(container.AutoOpsAgentImage, v)).
		WithEnv(autoopsEnvVars(policy, es)...).
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

	return common_deployment.New(common_deployment.Params{
		Name:      autoopsv1alpha1.Deployment(policy.GetName(), es),
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
	configMapName := autoopsv1alpha1.Config(policy.GetName(), es)
	var configMap corev1.ConfigMap
	configMapKey := types.NamespacedName{Namespace: policy.Namespace, Name: configMapName}
	if err := c.Get(ctx, configMapKey, &configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("ConfigMap %s not found: %w", configMapName, err)
		}
		return "", fmt.Errorf("failed to get ConfigMap %s: %w", configMapName, err)
	}

	if configData, ok := configMap.Data[autoOpsESConfigFileName]; ok {
		_, _ = configHash.Write([]byte(configData))
	}

	// Hash secret values from autoops-secret
	autoopsSecretNSN := types.NamespacedName{Namespace: autoOpsConfigurationSecretNamespace(policy), Name: policy.Spec.AutoOpsRef.SecretName}
	var autoopsSecret corev1.Secret
	if err := c.Get(ctx, autoopsSecretNSN, &autoopsSecret); err != nil {
		return "", fmt.Errorf("failed to get autoops configuration secret %s: %w", autoopsSecretNSN.String(), err)
	}

	// Hash secret keys, including optional keys. There's no code here to handle missing keys as:
	// 1. The optional keys are included here.
	//   1a. cloud-connected-mode-api-url is optional, and is only required if connecting to an environment such as non-production or air-gapped.
	// 2. The required keys are already validated in the controller, so they should always be present.
	keys := []string{autoOpsToken, autoOpsOTelURL, ccmAPIKey, ccmAPIURL}
	for _, key := range keys {
		if data, ok := autoopsSecret.Data[key]; ok {
			_, _ = configHash.Write(data)
		}
	}

	// // Hash ES API key secret
	esAPIKeySecretName := autoopsv1alpha1.APIKeySecret(policy.GetName(), k8s.ExtractNamespacedName(&es))
	esAPIKeySecretKey := types.NamespacedName{Namespace: policy.Namespace, Name: esAPIKeySecretName}
	var esAPIKeySecret corev1.Secret
	if err := c.Get(ctx, esAPIKeySecretKey, &esAPIKeySecret); err != nil {
		return "", fmt.Errorf("failed to get ES API key secret %s: %w", esAPIKeySecretName, err)
	}

	// // This data may not exist on initial reconciliation, so we don't return an error if it's missing.
	// // This should resolve itself on the next reconciliation after the API key is created.
	if apiKeyData, ok := esAPIKeySecret.Data[apiKeySecretKey]; ok {
		_, _ = configHash.Write(apiKeyData)
	}

	return fmt.Sprint(configHash.Sum32()), nil
}

// autoopsEnvVars returns the environment variables for the AutoOps deployment
// that reference values from the autoops-secret and the ES elastic user secret.
func autoopsEnvVars(policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) []corev1.EnvVar {
	esService := services.InternalServiceURL(es)
	return []corev1.EnvVar{
		{
			Name: "AUTOOPS_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: policy.Spec.AutoOpsRef.SecretName,
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
						Name: policy.Spec.AutoOpsRef.SecretName,
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
						Name: autoopsv1alpha1.APIKeySecret(policy.GetName(), k8s.ExtractNamespacedName(&es)),
					},
					Key:      apiKeySecretKey,
					Optional: ptr.To(false),
				},
			},
		},
		{
			Name: "ELASTIC_CLOUD_CONNECTED_MODE_API_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: policy.Spec.AutoOpsRef.SecretName,
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
						Name: policy.Spec.AutoOpsRef.SecretName,
					},
					Key:      "cloud-connected-mode-api-url",
					Optional: ptr.To(true),
				},
			},
		},
	}
}
