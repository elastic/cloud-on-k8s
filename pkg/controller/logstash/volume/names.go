// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

// Default values for the volume name and paths
const (
	LogstashDataVolumeName = "logstash-data"
	LogstashDataMountPath  = "/usr/share/logstash/data"

	LogstashLogsVolumeName = "logstash-logs"
	LogstashLogsMountPath  = "/usr/share/logstash/logs"

	ConfigVolumeName = "config"
	ConfigMountPath  = "/usr/share/logstash/config"

	InitContainerConfigVolumeMountPath = "/mnt/elastic-internal/logstash-config-local"
	// InternalConfigVolumeName is a volume which contains the generated configuration.
	InternalConfigVolumeName        = "elastic-internal-logstash-config"
	InternalConfigVolumeMountPath   = "/mnt/elastic-internal/logstash-config"
	InternalPipelineVolumeName      = "elastic-internal-logstash-pipeline"
	InternalPipelineVolumeMountPath = "/mnt/elastic-internal/logstash-pipeline"
)
