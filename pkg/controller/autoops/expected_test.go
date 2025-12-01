// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
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
		},
	}

	type args struct {
		autoops autoopsv1alpha1.AutoOpsAgentPolicy
		es      esv1.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		want    appsv1.Deployment
		wantErr bool
	}{
		{
			name: "default deployment params",
			args: args{
				autoops: autoopsFixture,
				es:      esFixture,
			},
			want:    expectedDeployment(autoopsFixture, esFixture),
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
			want:    appsv1.Deployment{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileAutoOpsAgentPolicy{}
			got, err := r.deploymentParams(tt.args.autoops, tt.args.es)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileAutoOpsAgentPolicy.deploymentParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if !cmp.Equal(got, tt.want) {
					t.Errorf("ReconcileAutoOpsAgentPolicy.deploymentParams() diff = %v", cmp.Diff(got, tt.want))
				}
			}
		})
	}
}

func expectedDeployment(autoops autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) appsv1.Deployment {
	v, _ := version.Parse(autoops.Spec.Version)
	labels := map[string]string{
		commonv1.TypeLabelName:        "autoops-agent",
		"autoops.k8s.elastic.co/name": autoops.Name,
	}

	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fmt.Sprintf("%s-autoops-deploy", es.Name),
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"autoops.k8s.elastic.co/name": autoops.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: nil,
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "config-volume",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "autoops-es-config",
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
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-volume",
									MountPath: "/mnt/config",
									ReadOnly:  true,
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
								{
									Name: "AUTOOPS_TEMP_RESOURCE_ID",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "autoops-secret",
											},
											Key: "temp-resource-id",
										},
									},
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
									Name: "ELASTICSEARCH_READ_API_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "autoops-secret",
											},
											Key: "es-api-key",
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
								ReadOnlyRootFilesystem: ptr.To(true),
								RunAsNonRoot:           ptr.To(true),
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
