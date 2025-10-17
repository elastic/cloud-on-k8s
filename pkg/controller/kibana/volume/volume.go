// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

const (
	// DataVolumeName is the name of the volume that holds the Kibana data
	DataVolumeName = "kibana-data"
	// DataVolumeMountPath is the path where the Kibana data is mounted in the Kibana container
	DataVolumeMountPath = "/usr/share/kibana/data"
	// PluginsVolumeName is the name of the volume that holds the Kibana plugins
	PluginsVolumeName = "kibana-plugins"
	// PluginsVolumeMountPath is the path where the Kibana plugins are mounted in the Kibana container
	PluginsVolumeMountPath = "/usr/share/kibana/plugins"
	// PluginsVolumeInternalMountPath is the path where the Kibana plugins are mounted in the init container
	PluginsVolumeInternalMountPath = "/mnt/elastic-internal/kibana-plugins-local"
	// LogsVolumeName is the name of the volume that holds the Kibana logs
	LogsVolumeName = "kibana-logs"
	// LogsVolumeMountPath is the path where the Kibana logs are mounted in the Kibana container
	LogsVolumeMountPath = "/usr/share/kibana/logs"
	// TempVolumeName is the name of the volume that holds the temporary files
	TempVolumeName = "temp-volume"
	// TempVolumeMountPath is the path where the temporary files are mounted in the Kibana container
	TempVolumeMountPath = "/tmp"
	// ScriptsVolumeName is the name of the volume that holds the Kibana scripts for the init container
	ScriptsVolumeName = "kibana-scripts"
	// ScriptsVolumeMountPath is the path where the Kibana scripts are mounted in the init container
	ScriptsVolumeMountPath = "/mnt/elastic-internal/scripts"
	// InitConfigContainerName is the name of the container that initializes the configuration
	InitContainerName = "elastic-internal-init"
	// ConfigVolumeName is the name of the volume that holds the Kibana configuration
	ConfigVolumeName = "elastic-internal-kibana-config-local"
	// ConfigVolumeMountPath is the path where the Kibana configuration is mounted in the Kibana container
	ConfigVolumeMountPath = "/usr/share/kibana/config"
	// InitContainerConfigVolumeMountPath is the path where the Kibana configuration is mounted in the init container
	InitContainerConfigVolumeMountPath = "/mnt/elastic-internal/kibana-config-local"
	// InternalConfigVolumeName is a volume which contains the generated configuration.
	InternalConfigVolumeName = "elastic-internal-kibana-config"
	// InternalConfigVolumeMountPath is the path where the generated configuration is mounted in the Kibana init container
	InternalConfigVolumeMountPath = "/mnt/elastic-internal/kibana-config"
	// EPRCACertPath is the path to the EPR CA certificate file
	EPRCACertPath = "/usr/share/kibana/config/epr-certs/ca.crt"
)
