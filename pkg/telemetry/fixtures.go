// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package telemetry

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
)

type TemplateData struct {
	ElasticsearchTemplateData
}

type ElasticsearchTemplateData struct {
	ResourceCount               int32
	StackMonitoringLogsCount    int32
	StackMonitoringMetricsCount int32
	PodCount                    int32

	*NodeLabelsTemplateData
}

type NodeLabelsTemplateData struct {
	ResourceWithNodeLabelsCount int32
	DistinctNodeLabelsCount     int32
}

func renderExpectedTemplate(t *testing.T, data TemplateData) []byte {
	t.Helper()
	tmpl, err := template.New("test").Parse(expectedTelemetryTemplate)
	require.NoError(t, err)
	tplBuffer := bytes.Buffer{}
	require.NoError(t, tmpl.Execute(&tplBuffer, data))
	return tplBuffer.Bytes()
}

const expectedTelemetryTemplate = `eck:
  build:
    date: "2019-09-20T07:00:00Z"
    hash: b5316231
    snapshot: "true"
    version: 1.1.0
  custom_operator_namespace: true
  distribution: v1.16.13-gke.1
  distributionChannel: test-channel
  license:
    eck_license_level: basic
    enterprise_resource_units: "1"
    total_managed_memory: 3.22GB
  operator_uuid: 15039433-f873-41bd-b6e7-10ee3665cafa
  stats:
    agents:
      multiple_refs: 0
      pod_count: 0
      resource_count: 0
    apms:
      pod_count: 0
      resource_count: 0
    beats:
      auditbeat_count: 0
      filebeat_count: 0
      heartbeat_count: 0
      journalbeat_count: 0
      metricbeat_count: 0
      packetbeat_count: 0
      pod_count: 0
      resource_count: 0
    elasticsearches:
      autoscaled_resource_count: 0
 {{- if .ElasticsearchTemplateData.NodeLabelsTemplateData }}
      downward_node_labels:
        distinct_node_labels_count: {{ .ElasticsearchTemplateData.DistinctNodeLabelsCount }}
        resource_count: {{ .ElasticsearchTemplateData.ResourceWithNodeLabelsCount }}
{{- end }}
      helm_resource_count: 0
      pod_count: {{ .ElasticsearchTemplateData.PodCount }}
      resource_count: {{ .ElasticsearchTemplateData.ResourceCount }}
      stack_monitoring_logs_count: {{ .ElasticsearchTemplateData.StackMonitoringLogsCount }}
      stack_monitoring_metrics_count: {{ .ElasticsearchTemplateData.StackMonitoringMetricsCount }}
    enterprisesearches:
      pod_count: 0
      resource_count: 0
    kibanas:
      helm_resource_count: 0
      pod_count: 0
      resource_count: 1
    logstashes:
      pipeline_count: 0
      pipeline_ref_count: 0
      pod_count: 0
      resource_count: 0
      service_count: 0
      stack_monitoring_logs_count: 0
      stack_monitoring_metrics_count: 0
    maps:
      pod_count: 0
      resource_count: 0
    stackconfigpolicies:
      configured_resources_count: 0
      resource_count: 0
      settings:
        cluster_settings_count: 0
        component_templates_count: 0
        composable_index_templates_count: 0
        index_lifecycle_policies_count: 0
        ingest_pipelines_count: 0
        role_mappings_count: 0
        snapshot_lifecycle_policies_count: 0
        snapshot_repositories_count: 0
`
