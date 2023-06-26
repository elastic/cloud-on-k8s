// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validations

import (
	"testing"

	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name  string
		es    esv1.Elasticsearch
		isErr bool
	}{
		{
			name: "without monitoring",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.13.1",
				},
			},
			isErr: false,
		},
		{
			name: "with monitoring",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.0",
					Monitoring: commonv1.Monitoring{
						Metrics: commonv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
						Logs: commonv1.LogsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
					},
				},
			},
			isErr: false,
		},
		{
			name: "with not supported version",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.13.1",
					Monitoring: commonv1.Monitoring{
						Metrics: commonv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
					},
				},
			},
			isErr: true,
		},
		{
			name: "with not only one elasticsearch ref for metrics",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.0",
					Monitoring: commonv1.Monitoring{
						Metrics: commonv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{
								{Name: "m1", Namespace: "b"},
								{Name: "m2", Namespace: "c"},
							},
						},
					},
				},
			},
			isErr: true,
		},
		{
			name: "with not only one elasticsearch ref for logs",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.0",
					Monitoring: commonv1.Monitoring{
						Logs: commonv1.LogsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{
								{Name: "m1", Namespace: "b"},
								{Name: "m2", Namespace: "c"},
							},
						},
					},
				},
			},
			isErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(&tc.es, tc.es.Spec.Version, MinStackVersion)
			if len(err) > 0 {
				require.True(t, tc.isErr)
			} else {
				require.False(t, tc.isErr)
			}
		})
	}
}
