// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func autoOpsContainerResources(pod corev1.PodTemplateSpec) (corev1.ResourceRequirements, bool) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == autoopsv1alpha1.AutoOpsAgentContainerName {
			return pod.Spec.Containers[i].Resources, true
		}
	}
	return corev1.ResourceRequirements{}, false
}

func TestAutoOpsResources(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "es-1", Namespace: "ns-1"},
		Spec:       esv1.ElasticsearchSpec{Version: "9.2.4"},
	}
	base := autoopsv1alpha1.AutoOpsAgentPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-1", Namespace: "ns-1"},
		Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
			Version:    "9.2.4",
			AutoOpsRef: autoopsv1alpha1.AutoOpsRef{SecretName: "config-secret"},
		},
	}

	for _, tt := range []struct {
		name   string
		mutate func(policy *autoopsv1alpha1.AutoOpsAgentPolicy)
		assert func(t *testing.T, got corev1.ResourceRequirements)
	}{
		{
			name:   "defaults applied when resources field and podTemplate are both unset",
			mutate: func(*autoopsv1alpha1.AutoOpsAgentPolicy) {},
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, defaultResources, got)
			},
		},
		{
			name: "resources field sets cpu and memory on autoops-agent container",
			mutate: func(p *autoopsv1alpha1.AutoOpsAgentPolicy) {
				p.Spec.Resources = commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						CPU:    ptr.To(resource.MustParse("250m")),
						Memory: ptr.To(resource.MustParse("512Mi")),
					},
					Limits: commonv1.ResourceAllocations{
						CPU:    ptr.To(resource.MustParse("500m")),
						Memory: ptr.To(resource.MustParse("1Gi")),
					},
				}
			},
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("250m"), got.Requests[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("512Mi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("500m"), got.Limits[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("1Gi"), got.Limits[corev1.ResourceMemory])
			},
		},
		{
			name: "resources field overrides podTemplate cpu and memory",
			mutate: func(p *autoopsv1alpha1.AutoOpsAgentPolicy) {
				p.Spec.PodTemplate = corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: autoopsv1alpha1.AutoOpsAgentContainerName,
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("200Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("200m"),
										corev1.ResourceMemory: resource.MustParse("200Mi"),
									},
								},
							},
						},
					},
				}
				p.Spec.Resources = commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						Memory: ptr.To(resource.MustParse("1Gi")),
					},
					Limits: commonv1.ResourceAllocations{
						Memory: ptr.To(resource.MustParse("1Gi")),
					},
				}
			},
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("1Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("1Gi"), got.Limits[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("100m"), got.Requests[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("200m"), got.Limits[corev1.ResourceCPU])
			},
		},
		{
			name: "podTemplate-only resources work without resources field",
			mutate: func(p *autoopsv1alpha1.AutoOpsAgentPolicy) {
				p.Spec.PodTemplate = corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: autoopsv1alpha1.AutoOpsAgentContainerName,
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("800Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("800Mi"),
									},
								},
							},
						},
					},
				}
			},
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("800Mi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("800Mi"), got.Limits[corev1.ResourceMemory])
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			policy := *base.DeepCopy()
			tt.mutate(&policy)
			r := &AgentPolicyReconciler{Client: k8s.NewFakeClient()}
			deploy, err := r.buildDeployment("test-hash", policy, es)
			require.NoError(t, err)
			res, ok := autoOpsContainerResources(deploy.Spec.Template)
			require.True(t, ok, "autoops-agent container not found")
			tt.assert(t, res)
		})
	}
}
