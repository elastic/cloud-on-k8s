// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"

const (
	MetricbeatConfigKey       = "metricbeat.yml"
	MetricbeatConfigMapSuffix = "metricbeat-config"

	FilebeatConfigKey       = "filebeat.yml"
	FilebeatConfigMapSuffix = "filebeat-config"
)

var (
	EsSourceURLEnvVarKey      = "ES_SOURCE_URL"
	EsSourceURLEnvVarValue    = "https://localhost:9200"
	EsSourceUsernameEnvVarKey = "ES_SOURCE_USERNAME"
	EsSourcePasswordEnvVarKey = "ES_SOURCE_PASSWORD" //nolint:gosec
	EsTargetURLEnvVarKey      = "ES_TARGET_URL"
	EsTargetUsernameEnvVarKey = "ES_TARGET_USERNAME"
	EsTargetPasswordEnvVarKey = "ES_TARGET_PASSWORD" //nolint:gosec

	// MetricbeatConfig is a static configuration for Metricbeat to collect monitoring data about Elasticsearch
	MetricbeatConfig = `metricbeat.modules:
- module: elasticsearch
  metricsets:
    - ccr
    - cluster_stats
    - enrich
    - index
    - index_recovery
    - index_summary
    - ml_job
    - node_stats
    - shard
  period: 10s
  xpack.enabled: true
  hosts: ["${ES_SOURCE_URL}"]
  username: ${ES_SOURCE_USERNAME}
  password: ${ES_SOURCE_PASSWORD}
  ssl.certificate_authorities: ["/mnt/es/monitoring/metrics/source/ca.crt"]
  ssl.verification_mode: "certificate"

processors:
  - add_cloud_metadata: {}
  - add_host_metadata: {}

output.elasticsearch:
  hosts: ['${ES_TARGET_URL}']
  username: ${ES_TARGET_USERNAME}
  password: ${ES_TARGET_PASSWORD}
  ssl.certificate_authorities: ["/mnt/es/monitoring/metrics/target/ca.crt"]`

	// FilebeatConfig is a static configuration for Filebeat to collect Elasticsearch logs
	FilebeatConfig = `filebeat.modules:
# https://www.elastic.co/guide/en/beats/filebeat/current/filebeat-module-elasticsearch.html
- module: elasticsearch
  server:
    enabled: true
    var.paths:
      - /usr/share/elasticsearch/logs/*_server.json
    close_timeout: 2h
    fields_under_root: true
  gc:
    enabled: true
    var.paths:
      - /usr/share/elasticsearch/logs/gc.log.[0-9]*
      - /usr/share/elasticsearch/logs/gc.log
      - /usr/share/elasticsearch/logs/gc.output.[0-9]*
      - /usr/share/elasticsearch/logs/gc.output
    close_timeout: 2h
    fields_under_root: true
  audit:
    enabled: true
    var.paths:
      - /usr/share/elasticsearch/logs/*_audit.json
    close_timeout: 2h
    fields_under_root: true
  slowlog:
    enabled: true
    var.paths:
      - /usr/share/elasticsearch/logs/*_index_search_slowlog.json
      - /usr/share/elasticsearch/logs/*_index_indexing_slowlog.json
    close_timeout: 2h
    fields_under_root: true
  deprecation:
    enabled: true
    var.paths:
      - /usr/share/elasticsearch/logs/*_deprecation.json
    close_timeout: 2h
    fields_under_root: true

processors:
  - add_cloud_metadata: {}
  - add_host_metadata: {}

#setup.dashboards.enabled: true
#setup.kibana:
  #host: '${ES_TARGET_URL}'
  #username: ${ES_TARGET_USERNAME}
  #password: ${ES_TARGET_PASSWORD}

output.elasticsearch:
  hosts: ['${ES_TARGET_URL}']
  username: ${ES_TARGET_USERNAME}
  password: ${ES_TARGET_PASSWORD}
  ssl.certificate_authorities: ["/mnt/es/monitoring/logs/target/ca.crt"]`
)

func MetricbeatConfigMapName(es esv1.Elasticsearch) string {
	return esv1.ESNamer.Suffix(es.Name, MetricbeatConfigMapSuffix)
}

func FilebeatConfigMapName(es esv1.Elasticsearch) string {
	return esv1.ESNamer.Suffix(es.Name, FilebeatConfigMapSuffix)
}
