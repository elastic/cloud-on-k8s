// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConfigVolumeName(t *testing.T) {
	name := configVolumeName(
		"extremely-long-and-unwieldy-name-that-exceeds-the-limit",
		"metricbeat",
	)
	assert.LessOrEqual(t, len(name), maxVolumeNameLength)
	assert.Equal(t, "extremely-long-and-unwieldy-name-that-exceeds-metricbeat-config", name)
}

func TestCAVolumeName(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample",
			Namespace: "aerospace",
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "7.14.0",
			Monitoring: esv1.Monitoring{
				Metrics: esv1.MetricsMonitoring{
					ElasticsearchRefs: []commonv1.ObjectSelector{{
						Name:      "extremely-long-and-unwieldy-name-that-exceeds-the-limit",
						Namespace: "extremely-long-and-unwieldy-namespace-that-exceeds-the-limit"}},
				},
				Logs: esv1.LogsMonitoring{
					ElasticsearchRefs: []commonv1.ObjectSelector{{
						Name:      "extremely-long-and-unwieldy-name-that-exceeds-the-limit",
						Namespace: "extremely-long-and-unwieldy-namespace-that-exceeds-the-limit"}},
				},
			},
		},
	}

	name := caVolumeName(es.GetMonitoringMetricsAssociation()[0])
	assert.LessOrEqual(t, len(name), maxVolumeNameLength)
	assert.Equal(t, "es-monitoring-954c60-ca", name)

	name = caVolumeName(es.GetMonitoringLogsAssociation()[0])
	assert.LessOrEqual(t, len(name), maxVolumeNameLength)
	assert.Equal(t, "es-monitoring-954c60-ca", name)

	es.Spec.Monitoring.Logs.ElasticsearchRefs[0].Name = "another-name"
	newName := caVolumeName(es.GetMonitoringLogsAssociation()[0])
	assert.NotEqual(t, name, newName)
	assert.Equal(t, "es-monitoring-ae0f57-ca", newName)
}
