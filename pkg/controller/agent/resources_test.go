// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"
	"hash/fnv"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func agentContainerResources(pod corev1.PodTemplateSpec) (corev1.ResourceRequirements, bool) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == ContainerName {
			return pod.Spec.Containers[i].Resources, true
		}
	}
	return corev1.ResourceRequirements{}, false
}

func buildAgentStandalonePodTemplate(t *testing.T, agent agentv1alpha1.Agent) corev1.PodTemplateSpec {
	t.Helper()
	configSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigSecretName(agent.Name),
			Namespace: agent.Namespace,
		},
	}
	params := Params{
		Context:      context.Background(),
		Client:       k8s.NewFakeClient(&configSecret),
		Agent:        agent,
		AgentVersion: version.MustParse(agent.Spec.Version),
	}
	got, err := buildPodTemplate(params, nil, EnrollmentAPIKey{}, fnv.New32a(), "")
	require.NoError(t, err)
	return got
}

func buildAgentFleetPodTemplate(t *testing.T, agent agentv1alpha1.Agent) corev1.PodTemplateSpec {
	t.Helper()
	configSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigSecretName(agent.Name),
			Namespace: agent.Namespace,
		},
	}
	params := Params{
		Context:      context.Background(),
		Client:       k8s.NewFakeClient(&configSecret),
		Agent:        agent,
		AgentVersion: version.MustParse(agent.Spec.Version),
	}
	got, err := buildPodTemplate(params, fleetCertsFixture, EnrollmentAPIKey{}, fnv.New32a(), "")
	require.NoError(t, err)
	return got
}

func TestAgentStandaloneResources(t *testing.T) {
	base := agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-test", Namespace: "default"},
	}

	for _, tt := range []struct {
		name   string
		agent  agentv1alpha1.Agent
		assert func(t *testing.T, got corev1.ResourceRequirements)
	}{
		{
			name: "standalone: defaults applied when resources field is unset",
			agent: func() agentv1alpha1.Agent {
				a := base
				a.Spec = agentv1alpha1.AgentSpec{
					Version:   "8.17.0",
					DaemonSet: &agentv1alpha1.DaemonSetSpec{},
				}
				return a
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, defaultResources, got)
			},
		},
		{
			name: "standalone: resources field sets cpu and memory",
			agent: func() agentv1alpha1.Agent {
				a := base
				a.Spec = agentv1alpha1.AgentSpec{
					Version:   "8.17.0",
					DaemonSet: &agentv1alpha1.DaemonSetSpec{},
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{
							CPU:    ptr.To(resource.MustParse("200m")),
							Memory: ptr.To(resource.MustParse("512Mi")),
						},
						Limits: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("512Mi")),
						},
					},
				}
				return a
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("200m"), got.Requests[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("512Mi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("512Mi"), got.Limits[corev1.ResourceMemory])
			},
		},
		{
			name: "standalone: resources field overrides podTemplate cpu and memory",
			agent: func() agentv1alpha1.Agent {
				a := base
				a.Spec = agentv1alpha1.AgentSpec{
					Version: "8.17.0",
					DaemonSet: &agentv1alpha1.DaemonSetSpec{
						PodTemplate: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: ContainerName,
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceMemory: resource.MustParse("256Mi"),
												corev1.ResourceCPU:    resource.MustParse("100m"),
											},
											Limits: corev1.ResourceList{
												corev1.ResourceMemory: resource.MustParse("256Mi"),
											},
										},
									},
								},
							},
						},
					},
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("1Gi")),
						},
						Limits: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("1Gi")),
						},
					},
				}
				return a
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("1Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("1Gi"), got.Limits[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("100m"), got.Requests[corev1.ResourceCPU])
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := buildAgentStandalonePodTemplate(t, tt.agent)
			res, ok := agentContainerResources(pod)
			require.True(t, ok, "agent container not found")
			tt.assert(t, res)
		})
	}
}

func TestAgentFleetResources(t *testing.T) {
	base := agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-fleet-test", Namespace: "default"},
	}

	for _, tt := range []struct {
		name   string
		agent  agentv1alpha1.Agent
		assert func(t *testing.T, got corev1.ResourceRequirements)
	}{
		{
			name: "fleet: defaults applied when resources field is unset",
			agent: func() agentv1alpha1.Agent {
				a := base
				a.Spec = agentv1alpha1.AgentSpec{
					Version:   "8.17.0",
					Mode:      agentv1alpha1.AgentFleetMode,
					DaemonSet: &agentv1alpha1.DaemonSetSpec{},
				}
				return a
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, defaultFleetResources, got)
			},
		},
		{
			name: "fleet: resources field overrides fleet defaults",
			agent: func() agentv1alpha1.Agent {
				a := base
				a.Spec = agentv1alpha1.AgentSpec{
					Version:   "8.17.0",
					Mode:      agentv1alpha1.AgentFleetMode,
					DaemonSet: &agentv1alpha1.DaemonSetSpec{},
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("2Gi")),
						},
						Limits: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("2Gi")),
						},
					},
				}
				return a
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("2Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("2Gi"), got.Limits[corev1.ResourceMemory])
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := buildAgentFleetPodTemplate(t, tt.agent)
			res, ok := agentContainerResources(pod)
			require.True(t, ok, "agent container not found in fleet mode")
			tt.assert(t, res)
		})
	}
}
