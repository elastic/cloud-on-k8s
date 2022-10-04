// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
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
			Monitoring: commonv1.Monitoring{
				Metrics: commonv1.MetricsMonitoring{
					ElasticsearchRefs: []commonv1.ObjectSelector{{
						Name:      "extremely-long-and-unwieldy-name-that-exceeds-the-limit",
						Namespace: "extremely-long-and-unwieldy-namespace-that-exceeds-the-limit"}},
				},
				Logs: commonv1.LogsMonitoring{
					ElasticsearchRefs: []commonv1.ObjectSelector{{
						Name:      "extremely-long-and-unwieldy-name-that-exceeds-the-limit",
						Namespace: "extremely-long-and-unwieldy-namespace-that-exceeds-the-limit"}},
				},
			},
		},
	}

	name := caVolumeName(monitoring.GetMetricsAssociation(&es)[0])
	assert.LessOrEqual(t, len(name), maxVolumeNameLength)
	assert.Equal(t, "es-monitoring-954c60-ca", name)

	name = caVolumeName(monitoring.GetLogsAssociation(&es)[0])
	assert.LessOrEqual(t, len(name), maxVolumeNameLength)
	assert.Equal(t, "es-monitoring-954c60-ca", name)

	es.Spec.Monitoring.Logs.ElasticsearchRefs[0].Name = "another-name"
	newName := caVolumeName(monitoring.GetLogsAssociation(&es)[0])
	assert.NotEqual(t, name, newName)
	assert.Equal(t, "es-monitoring-ae0f57-ca", newName)
}
