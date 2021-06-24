// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestWithMonitoring(t *testing.T) {
	tests := []struct {
		name                   string
		es                     esv1.Elasticsearch
		containersLength       int
		esEnvVarsLength        int
		podVolumesLength       int
		beatVolumeMountsLength int
	}{
		{
			name: "without monitoring",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.1",
				},
			},
			containersLength: 1,
		},
		{
			name: "with metrics monitoring",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.1",
					Monitoring: esv1.Monitoring{
						Metrics: esv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
					},
				},
			},
			containersLength:       2,
			esEnvVarsLength:        0,
			podVolumesLength:       3,
			beatVolumeMountsLength: 3,
		},
		{
			name: "with logs monitoring",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.1",
					Monitoring: esv1.Monitoring{
						Logs: esv1.LogsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
					},
				},
			},
			containersLength:       2,
			esEnvVarsLength:        1,
			podVolumesLength:       2,
			beatVolumeMountsLength: 3,
		},
		{
			name: "with metrics and logs monitoring",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.1",
					Monitoring: esv1.Monitoring{
						Metrics: esv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
						Logs: esv1.LogsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
					},
				},
			},
			containersLength:       3,
			esEnvVarsLength:        1,
			podVolumesLength:       5,
			beatVolumeMountsLength: 3,
		},
		{
			name: "with metrics and logs monitoring with different es ref",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.1",
					Monitoring: esv1.Monitoring{
						Metrics: esv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
						Logs: esv1.LogsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m2", Namespace: "c"}},
						},
					},
				},
			},
			containersLength:       3,
			esEnvVarsLength:        1,
			podVolumesLength:       5,
			beatVolumeMountsLength: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := defaults.NewPodTemplateBuilder(corev1.PodTemplateSpec{}, esv1.ElasticsearchContainerName)
			builder, err := WithMonitoring(builder, tc.es)
			assert.NoError(t, err)
			assert.Equal(t, tc.containersLength, len(builder.PodTemplate.Spec.Containers))
			assert.Equal(t, tc.esEnvVarsLength, len(builder.PodTemplate.Spec.Containers[0].Env))
			assert.Equal(t, tc.podVolumesLength, len(builder.PodTemplate.Spec.Volumes))
			if IsMonitoringMetricsDefined(tc.es) {
				for _, c := range builder.PodTemplate.Spec.Containers {
					if c.Name == "metricbeat" {
						assert.Equal(t, tc.beatVolumeMountsLength, len(c.VolumeMounts))
					}
				}
			}
			if IsMonitoringLogsDefined(tc.es) {
				for _, c := range builder.PodTemplate.Spec.Containers {
					if c.Name == "filebeat" {
						assert.Equal(t, tc.beatVolumeMountsLength, len(c.VolumeMounts))
					}
				}
			}
		})
	}
}
