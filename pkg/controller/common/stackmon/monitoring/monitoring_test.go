// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package monitoring

import (
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/stretchr/testify/assert"
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
