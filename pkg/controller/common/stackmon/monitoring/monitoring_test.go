// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package monitoring

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
)

var (
	sampleEs          = esv1.Elasticsearch{}
	monitoringEsRef   = commonv1.ObjectSelector{Name: "monitoring", Namespace: "observability"}
	sampleMonitoredEs = esv1.Elasticsearch{
		Spec: esv1.ElasticsearchSpec{
			Monitoring: esv1.Monitoring{
				Metrics: esv1.MetricsMonitoring{
					ElasticsearchRefs: []commonv1.ObjectSelector{monitoringEsRef},
				},
			},
		},
	}
)

func TestIsReconcilable(t *testing.T) {
	tests := []struct {
		name string
		es   esv1.Elasticsearch
		want bool
	}{
		{
			name: "without monitoring",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.13.1",
				},
			},
			want: false,
		},
		{
			name: "with metrics monitoring defined but not configured",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Monitoring: esv1.Monitoring{
						Metrics: esv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "with metrics monitoring defined and configured",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Monitoring: esv1.Monitoring{
						Metrics: esv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
					},
				},
				AssocConfs: map[types.NamespacedName]commonv1.AssociationConf{
					types.NamespacedName{Name: "m1", Namespace: "b"}: {URL: "https://es.xyz", AuthSecretName: "-"},
				},
			},
			want: true,
		},
		{
			name: "with logs monitoring defined and configured",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Monitoring: esv1.Monitoring{
						Logs: esv1.LogsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
					},
				},
				AssocConfs: map[types.NamespacedName]commonv1.AssociationConf{
					types.NamespacedName{Name: "m1", Namespace: "b"}: {URL: "https://es.xyz", AuthSecretName: "-"},
				},
			},
			want: true,
		},
		{
			name: "with metrics and logs monitoring defined and partially configured",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Monitoring: esv1.Monitoring{
						Metrics: esv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
						Logs: esv1.LogsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m2", Namespace: "b"}},
						},
					},
				},
				AssocConfs: map[types.NamespacedName]commonv1.AssociationConf{
					types.NamespacedName{Name: "m1", Namespace: "b"}: {URL: "https://es.xyz", AuthSecretName: "-"},
				},
			},
			want: false,
		},
		{
			name: "with metrics and logs monitoring defined and partially configured",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Monitoring: esv1.Monitoring{
						Metrics: esv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
						Logs: esv1.LogsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m2", Namespace: "b"}},
						},
					},
				},
				AssocConfs: map[types.NamespacedName]commonv1.AssociationConf{
					types.NamespacedName{Name: "m1", Namespace: "b"}: {URL: "https://es.xyz", AuthSecretName: "-"},
				},
			},
			want: false,
		},
		{
			name: "with logs and metrics monitoring defined and configured",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Monitoring: esv1.Monitoring{
						Metrics: esv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
						Logs: esv1.LogsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
					},
				},
				AssocConfs: map[types.NamespacedName]commonv1.AssociationConf{
					types.NamespacedName{Name: "m1", Namespace: "b"}: {URL: "https://m1.xyz", AuthSecretName: "-"},
				},
			},
			want: true,
		},
		{
			name: "with distinct logs and metrics monitoring defined and configured",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Monitoring: esv1.Monitoring{
						Metrics: esv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m1", Namespace: "b"}},
						},
						Logs: esv1.LogsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{{Name: "m2", Namespace: "b"}},
						},
					},
				},
				AssocConfs: map[types.NamespacedName]commonv1.AssociationConf{
					types.NamespacedName{Name: "m1", Namespace: "b"}: {URL: "https://m1.xyz", AuthSecretName: "-"},
					types.NamespacedName{Name: "m2", Namespace: "b"}: {URL: "https://m2.xyz", AuthSecretName: "-"},
				},
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IsReconcilable(&tc.es)
			assert.NoError(t, err)
			if got != tc.want {
				t.Errorf("IsReconcilable() got = %v, want %v", got, tc.want)
				return
			}
		})
	}
}

func TestIsDefined(t *testing.T) {
	assert.False(t, IsDefined(&sampleEs))
	assert.True(t, IsDefined(&sampleMonitoredEs))
}

func TestIsMetricsDefined(t *testing.T) {
	assert.False(t, IsMetricsDefined(&sampleEs))
	assert.True(t, IsMetricsDefined(&sampleMonitoredEs))
}

func TestIsLogsDefined(t *testing.T) {
	assert.False(t, IsLogsDefined(&sampleEs))
	assert.False(t, IsLogsDefined(&sampleMonitoredEs))
}

func TestAreEsRefsDefined(t *testing.T) {
	assert.False(t, AreEsRefsDefined(sampleEs.Spec.Monitoring.Metrics.ElasticsearchRefs))
	assert.True(t, AreEsRefsDefined(sampleMonitoredEs.Spec.Monitoring.Metrics.ElasticsearchRefs))
	assert.False(t, AreEsRefsDefined(sampleMonitoredEs.Spec.Monitoring.Logs.ElasticsearchRefs))
}

func TestGetMetricsAssociation(t *testing.T) {
	assert.Equal(t, 0, len(GetMetricsAssociation(&sampleEs)))
	assert.Equal(t, 1, len(GetMetricsAssociation(&sampleMonitoredEs)))
}

func TestGetLogsAssociation(t *testing.T) {
	assert.Equal(t, 0, len(GetLogsAssociation(&sampleEs)))
	assert.Equal(t, 0, len(GetLogsAssociation(&sampleMonitoredEs)))
}
