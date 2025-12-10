// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"crypto/sha256"
	"fmt"
	"hash/fnv"
	"testing"

	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/pointer"
)

func TestReconcileAutoOpsAgentPolicy_deploymentParams(t *testing.T) {
	autoopsFixture := autoopsv1alpha1.AutoOpsAgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "autoops-elastic-agent",
			Namespace: "default",
		},
		Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
			Version: "9.1.0-SNAPSHOT",
		},
	}

	esFixture := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es-cluster",
			Namespace: "default",
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "9.1.0",
			HTTP: commonv1.HTTPConfig{
				TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{
						Disabled: true,
					},
				},
			},
		},
	}

	type args struct {
		autoops autoopsv1alpha1.AutoOpsAgentPolicy
		es      esv1.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "default deployment params",
			args: args{
				autoops: autoopsFixture,
				es:      esFixture,
			},
			wantErr: false,
		},
		{
			name: "invalid version",
			args: args{
				autoops: autoopsv1alpha1.AutoOpsAgentPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "autoops-elastic-agent",
						Namespace: "default",
					},
					Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
						Version: "invalid-version",
					},
				},
				es: esFixture,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We need a ConfigMap to calculate the config hash for the deployment.
			configData := "test-config-data"
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%s-%s", autoOpsESConfigMapName, tt.args.es.Namespace, tt.args.es.Name),
					Namespace: tt.args.autoops.Namespace,
				},
				Data: map[string]string{
					autoOpsESConfigFileName: configData,
				},
			}

			// We need the autoops-secret with all required keys to build the config hash
			autoopsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "autoops-secret",
					Namespace: tt.args.autoops.Namespace,
				},
				Data: map[string][]byte{
					"autoops-token":                []byte("test-autoops-token"),
					"autoops-otel-url":             []byte("https://test-otel-url"),
					"cloud-connected-mode-api-key": []byte("test-ccm-api-key"),
					"cloud-connected-mode-api-url": []byte("https://test-ccm-api-url"),
				},
			}

			// We need the ES API key secret as well to build the config hash
			esAPIKeySecretName := apiKeySecretNameFor(types.NamespacedName{Namespace: tt.args.es.Namespace, Name: tt.args.es.Name})
			esAPIKeySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      esAPIKeySecretName,
					Namespace: tt.args.autoops.Namespace,
				},
				Data: map[string][]byte{
					"api_key": []byte("test-es-api-key"),
				},
			}

			client := k8s.NewFakeClient(configMap, autoopsSecret, esAPIKeySecret)
			r := &ReconcileAutoOpsAgentPolicy{
				Client: client,
			}
			ctx := context.Background()
			got, err := r.deploymentParams(ctx, tt.args.autoops, tt.args.es)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileAutoOpsAgentPolicy.deploymentParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Calculate expected config hash to match what buildConfigHash computes
				expectedConfigHash := fnv.New32a()
				_, _ = expectedConfigHash.Write([]byte(configData))
				// Hash autoops-secret values
				_, _ = expectedConfigHash.Write([]byte("test-autoops-token"))
				_, _ = expectedConfigHash.Write([]byte("https://test-otel-url"))
				_, _ = expectedConfigHash.Write([]byte("test-ccm-api-key"))
				_, _ = expectedConfigHash.Write([]byte("https://test-ccm-api-url"))
				// Hash ES API key secret value
				_, _ = expectedConfigHash.Write([]byte("test-es-api-key"))
				expectedHashStr := fmt.Sprint(expectedConfigHash.Sum32())
				want := expectedDeployment(tt.args.autoops, tt.args.es, expectedHashStr)
				if !cmp.Equal(got, want) {
					t.Errorf("ReconcileAutoOpsAgentPolicy.deploymentParams() diff = %v", cmp.Diff(got, want))
				}
			}
		})
	}
}

func expectedDeployment(policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch, configHashValue string) appsv1.Deployment {
	v, _ := version.Parse(policy.Spec.Version)
	labels := map[string]string{
		commonv1.TypeLabelName:        "autoops-agent",
		"autoops.k8s.elastic.co/name": policy.GetName(),
	}

	annotations := map[string]string{
		configHashAnnotationName: configHashValue,
	}

	// Hash ES namespace and name to match the implementation
	esIdentifier := es.GetNamespace() + es.GetName()
	esHash := fmt.Sprintf("%x", sha256.Sum256([]byte(esIdentifier)))[0:6]
	name := AutoOpsNamer.Suffix(policy.GetName(), esHash)
	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   policy.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"autoops.k8s.elastic.co/name": policy.GetName(),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "config-volume",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: AutoOpsConfigNamer.Suffix(policy.GetName(), esHash),
									},
									DefaultMode: ptr.To(corev1.ConfigMapVolumeSourceDefaultMode),
									Optional:    ptr.To(false),
								},
							},
						},
					},
					AutomountServiceAccountToken: ptr.To(false),
					Containers: []corev1.Container{
						{
							Name:  "autoops-agent",
							Image: fmt.Sprintf("docker.elastic.co/elastic-agent/elastic-otel-collector-wolfi:%s", v.String()),
							Args: []string{
								"--config",
								"/mnt/config/autoops_es.yml",
							},
							Resources: defaultResources,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: int32(readinessProbePort),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-volume",
									MountPath: "/mnt/config",
									ReadOnly:  true,
								},
							},
							ReadinessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Port:   intstr.FromInt(readinessProbePort),
										Path:   "/health/status",
										Scheme: corev1.URISchemeHTTP,
									},
								},
							},
							Env: []corev1.EnvVar{
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
									Value: "http://es-cluster-es-internal-http.default.svc:9200",
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
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								Privileged:             ptr.To(false),
								ReadOnlyRootFilesystem: ptr.To(false),
								SeccompProfile: &corev1.SeccompProfile{
									Type: corev1.SeccompProfileTypeRuntimeDefault,
								},
							},
						},
					},
				},
			},
		},
	}
}

func Test_readinessProbe(t *testing.T) {
	tests := []struct {
		name string
		want corev1.Probe
	}{
		{
			name: "readiness probe configuration",
			want: corev1.Probe{
				FailureThreshold:    3,
				InitialDelaySeconds: 5,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				TimeoutSeconds:      5,
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Port:   intstr.FromInt(readinessProbePort),
						Path:   "/health/status",
						Scheme: corev1.URISchemeHTTP,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readinessProbe()
			if !cmp.Equal(got, tt.want) {
				t.Errorf("readinessProbe() diff = %v", cmp.Diff(got, tt.want))
			}
		})
	}
}

func Test_autoopsEnvVars(t *testing.T) {
	tests := []struct {
		name   string
		es     esv1.Elasticsearch
		policy autoopsv1alpha1.AutoOpsAgentPolicy
		want   []corev1.EnvVar
	}{
		{
			name: "Happy path",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "es-1",
					Namespace: "ns-1",
				},
			},
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-2",
				},
			},
			want: []corev1.EnvVar{
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
					Value: "https://es-1-es-internal-http.ns-1.svc:9200",
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
								Name: "es-1-ns-1-autoops-es-api-key",
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := autoopsEnvVars(tt.es)
			if !cmp.Equal(got, tt.want) {
				t.Errorf("autoopsEnvVars() diff = %v", cmp.Diff(got, tt.want))
			}
		})
	}
}
