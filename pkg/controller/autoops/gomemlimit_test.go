// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_goMemLimitFor(t *testing.T) {
	memory := func(quantity string) corev1.ResourceList {
		return corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(quantity)}
	}
	for _, tt := range []struct {
		name      string
		resources corev1.ResourceRequirements
		want      string
		wantOK    bool
	}{
		{
			name:      "default resources",
			resources: defaultResources,
			want:      "320MiB",
			wantOK:    true,
		},
		{
			name:      "limit is used over the request",
			resources: corev1.ResourceRequirements{Limits: memory("1Gi"), Requests: memory("512Mi")},
			want:      "819MiB",
			wantOK:    true,
		},
		{
			name:      "request is used when there is no limit",
			resources: corev1.ResourceRequirements{Requests: memory("512Mi")},
			want:      "409MiB",
			wantOK:    true,
		},
		{
			name: "request is used when limits only set cpu",
			resources: corev1.ResourceRequirements{
				Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
				Requests: memory("200Mi"),
			},
			want:   "160MiB",
			wantOK: true,
		},
		{
			name:      "decimal quantity is rounded down to a whole MiB",
			resources: corev1.ResourceRequirements{Limits: memory("1G")},
			want:      "762MiB",
			wantOK:    true,
		},
		{
			name:      "very large limit does not overflow",
			resources: corev1.ResourceRequirements{Limits: memory("4Ei")},
			want:      "3518437208883MiB",
			wantOK:    true,
		},
		{
			name:      "no resources",
			resources: corev1.ResourceRequirements{},
		},
		{
			name:      "empty limits and requests, as used with a LimitRange",
			resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}},
		},
		{
			name:      "zero limit and no request",
			resources: corev1.ResourceRequirements{Limits: memory("0")},
		},
		{
			name:      "limit too small for a whole MiB",
			resources: corev1.ResourceRequirements{Limits: memory("1Mi")},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := goMemLimitFor(tt.resources)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAutoOpsGoMemLimit(t *testing.T) {
	es := esv1.Elasticsearch{
		Name: "es-1", Namespace: "ns-1",
		Spec: esv1.ElasticsearchSpec{Version: "9.2.4"},
	}
	base := autoopsv1alpha1.AutoOpsAgentPolicy{
		Name: "policy-1", Namespace: "ns-1",
		Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
			Version:    "9.2.4",
			AutoOpsRef: autoopsv1alpha1.AutoOpsRef{SecretName: "config-secret"},
		},
	}
	podTemplateWith := func(c corev1.Container) corev1.PodTemplateSpec {
		c.Name = autoopsv1alpha1.AutoOpsAgentContainerName
		return corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{c}}}
	}

	for _, tt := range []struct {
		name   string
		mutate func(policy *autoopsv1alpha1.AutoOpsAgentPolicy)
		// want is the expected GOMEMLIMIT env var, nil when it must not be set.
		want *corev1.EnvVar
	}{
		{
			name:   "default resources",
			mutate: func(*autoopsv1alpha1.AutoOpsAgentPolicy) {},
			want:   &corev1.EnvVar{Name: "GOMEMLIMIT", Value: "320MiB"},
		},
		{
			name: "follows the memory limit of the resources field",
			mutate: func(p *autoopsv1alpha1.AutoOpsAgentPolicy) {
				p.Spec.Resources = commonv1.Resources{
					Requests: commonv1.ResourceAllocations{Memory: new(resource.MustParse("512Mi"))},
					Limits:   commonv1.ResourceAllocations{Memory: new(resource.MustParse("1Gi"))},
				}
			},
			want: &corev1.EnvVar{Name: "GOMEMLIMIT", Value: "819MiB"},
		},
		{
			name: "follows the memory limit of the podTemplate",
			mutate: func(p *autoopsv1alpha1.AutoOpsAgentPolicy) {
				p.Spec.PodTemplate = podTemplateWith(corev1.Container{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("800Mi")},
					},
				})
			},
			want: &corev1.EnvVar{Name: "GOMEMLIMIT", Value: "640MiB"},
		},
		{
			name: "resources field wins over the podTemplate",
			mutate: func(p *autoopsv1alpha1.AutoOpsAgentPolicy) {
				p.Spec.PodTemplate = podTemplateWith(corev1.Container{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("200Mi")},
					},
				})
				p.Spec.Resources = commonv1.Resources{
					Limits: commonv1.ResourceAllocations{Memory: new(resource.MustParse("2Gi"))},
				}
			},
			want: &corev1.EnvVar{Name: "GOMEMLIMIT", Value: "1638MiB"},
		},
		{
			name: "falls back to the memory request when there is no memory limit",
			mutate: func(p *autoopsv1alpha1.AutoOpsAgentPolicy) {
				p.Spec.PodTemplate = podTemplateWith(corev1.Container{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("500Mi")},
					},
				})
			},
			want: &corev1.EnvVar{Name: "GOMEMLIMIT", Value: "400MiB"},
		},
		{
			name: "not set when there is neither a memory limit nor a memory request",
			mutate: func(p *autoopsv1alpha1.AutoOpsAgentPolicy) {
				// Empty limits and requests disable the operator defaults, for example to rely on a LimitRange.
				p.Spec.PodTemplate = podTemplateWith(corev1.Container{
					Resources: corev1.ResourceRequirements{
						Limits:   corev1.ResourceList{},
						Requests: corev1.ResourceList{},
					},
				})
			},
			want: nil,
		},
		{
			name: "user-provided value wins",
			mutate: func(p *autoopsv1alpha1.AutoOpsAgentPolicy) {
				p.Spec.PodTemplate = podTemplateWith(corev1.Container{
					Env: []corev1.EnvVar{{Name: "GOMEMLIMIT", Value: "250MiB"}},
				})
			},
			want: &corev1.EnvVar{Name: "GOMEMLIMIT", Value: "250MiB"},
		},
		{
			name: "user-provided value wins when the limit is changed too",
			mutate: func(p *autoopsv1alpha1.AutoOpsAgentPolicy) {
				p.Spec.PodTemplate = podTemplateWith(corev1.Container{
					Env: []corev1.EnvVar{{Name: "GOMEMLIMIT", Value: "off"}},
				})
				p.Spec.Resources = commonv1.Resources{
					Limits: commonv1.ResourceAllocations{Memory: new(resource.MustParse("1Gi"))},
				}
			},
			want: &corev1.EnvVar{Name: "GOMEMLIMIT", Value: "off"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			policy := *base.DeepCopy()
			tt.mutate(&policy)
			r := &AgentPolicyReconciler{Client: k8s.NewFakeClient()}
			deploy, err := r.buildDeployment("test-hash", policy, es)
			require.NoError(t, err)

			var got []corev1.EnvVar
			for _, c := range deploy.Spec.Template.Spec.Containers {
				if c.Name != autoopsv1alpha1.AutoOpsAgentContainerName {
					continue
				}
				for _, env := range c.Env {
					if env.Name == "GOMEMLIMIT" {
						got = append(got, env)
					}
				}
			}
			if tt.want == nil {
				require.Empty(t, got)
				return
			}
			require.Equal(t, []corev1.EnvVar{*tt.want}, got)
		})
	}
}
