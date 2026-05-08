// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"hash/fnv"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/logstash/configs"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func logstashContainerResources(pod corev1.PodTemplateSpec) (corev1.ResourceRequirements, bool) {
	return GetLogstashContainer(pod.Spec).Resources, GetLogstashContainer(pod.Spec) != nil
}

func buildLogstashPodTemplate(t *testing.T, ls logstashv1alpha1.Logstash) corev1.PodTemplateSpec {
	t.Helper()
	// buildPodTemplate fetches the HTTP certs secret to compute TLS hash — provide a minimal one.
	httpCertsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ls.Name + "-ls-http-certs-internal",
			Namespace: ls.Namespace,
		},
	}
	params := Params{
		Context:         context.Background(),
		Client:          k8s.NewFakeClient(&httpCertsSecret),
		Logstash:        ls,
		APIServerConfig: configs.APIServer{},
	}
	configHash := fnv.New32a()
	got, err := buildPodTemplate(params, configHash)
	require.NoError(t, err)
	return got
}

func TestLogstashResources(t *testing.T) {
	base := logstashv1alpha1.Logstash{
		ObjectMeta: metav1.ObjectMeta{Name: "logstash-test", Namespace: "default"},
	}

	for _, tt := range []struct {
		name   string
		ls     logstashv1alpha1.Logstash
		assert func(t *testing.T, got corev1.ResourceRequirements)
	}{
		{
			name: "defaults applied when resources field is unset",
			ls: func() logstashv1alpha1.Logstash {
				l := base
				l.Spec = logstashv1alpha1.LogstashSpec{Version: "8.17.0"}
				return l
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, DefaultResources, got)
			},
		},
		{
			name: "resources field sets cpu and memory",
			ls: func() logstashv1alpha1.Logstash {
				l := base
				l.Spec = logstashv1alpha1.LogstashSpec{
					Version: "8.17.0",
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{
							CPU:    ptr.To(resource.MustParse("1")),
							Memory: ptr.To(resource.MustParse("2Gi")),
						},
						Limits: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("2Gi")),
						},
					},
				}
				return l
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("1"), got.Requests[corev1.ResourceCPU])
				require.Equal(t, resource.MustParse("2Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("2Gi"), got.Limits[corev1.ResourceMemory])
			},
		},
		{
			name: "resources field overrides podTemplate cpu and memory",
			ls: func() logstashv1alpha1.Logstash {
				l := base
				l.Spec = logstashv1alpha1.LogstashSpec{
					Version: "8.17.0",
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: logstashv1alpha1.LogstashContainerName,
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("1Gi"),
											corev1.ResourceCPU:    resource.MustParse("500m"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceMemory: resource.MustParse("1Gi"),
										},
									},
								},
							},
						},
					},
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("4Gi")),
						},
						Limits: commonv1.ResourceAllocations{
							Memory: ptr.To(resource.MustParse("4Gi")),
						},
					},
				}
				return l
			}(),
			assert: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, resource.MustParse("4Gi"), got.Requests[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("4Gi"), got.Limits[corev1.ResourceMemory])
				require.Equal(t, resource.MustParse("500m"), got.Requests[corev1.ResourceCPU])
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := buildLogstashPodTemplate(t, tt.ls)
			res, ok := logstashContainerResources(pod)
			require.True(t, ok, "logstash container not found")
			tt.assert(t, res)
		})
	}
}
