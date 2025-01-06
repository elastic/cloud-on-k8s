// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

const (
	DataVolumeName               = "kibana-data"
	DataVolumeMountPath          = "/usr/share/kibana/data"
	PluginsVolumeName            = "kibana-plugins"
	PluginsVolumeMountPath       = "/usr/share/kibana/plugins"
	LogsVolumeName               = "kibana-logs"
	LogsVolumeMountPath          = "/usr/share/kibana/logs"
	TempVolumeName               = "temp-volume"
	TempVolumeMountPath          = "/tmp"
	KibanaBasePathEnvName        = "SERVER_BASEPATH"
	KibanaRewriteBasePathEnvName = "SERVER_REWRITEBASEPATH"
	ScriptsVolumeMountPath       = "/mnt/elastic-internal/scripts"
	// PrepareFilesystemContainerName is the name of the container that prepares the filesystem
	// PrepareFilesystemContainerName = "elastic-internal-init-filesystem"
	// KibanaPluginsVolumeName is the name of the volume that holds the Kibana plugins
	KibanaPluginsVolumeName = "kibana-plugins"
	// KibanaPluginsInternalMountPath is the path where the Kibana plugins are mounted in the init container
	KibanaPluginsInternalMountPath = "/mnt/elastic-internal/kibana-plugins-local"
	// KibanaPluginsMountPath is the path where the Kibana plugins are mounted in the Kibana container
	// KibanaPluginsMountPath = "/usr/share/kibana/plugins"

	// InitConfigContainerName is the name of the container that initializes the configuration
	InitContainerName = "elastic-internal-init"
	// ConfigVolumeName is the name of the volume that holds the Kibana configuration
	ConfigVolumeName                   = "elastic-internal-kibana-config-local"
	ConfigVolumeMountPath              = "/usr/share/kibana/config"
	InitContainerConfigVolumeMountPath = "/mnt/elastic-internal/kibana-config-local"

	// InternalConfigVolumeName is a volume which contains the generated configuration.
	InternalConfigVolumeName      = "elastic-internal-kibana-config"
	InternalConfigVolumeMountPath = "/mnt/elastic-internal/kibana-config"
)
