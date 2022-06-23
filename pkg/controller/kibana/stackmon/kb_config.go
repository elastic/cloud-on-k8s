// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"path/filepath"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
)

var (
	MonitoringKibanaCollectionEnabled = "monitoring.kibana.collection.enabled"

	LoggingAppendersJSONFileAppenderType       = "logging.appenders.rolling-file.type"
	LoggingAppendersJSONFileAppenderFilename   = "logging.appenders.rolling-file.fileName"
	LoggingAppendersJSONFileAppenderLayoutType = "logging.appenders.rolling-file.layout.type"
	LoggingAppendersJSONFileAppenderPolicyType = "logging.appenders.rolling-file.policy.type"
	LoggingAppendersJSONFileAppenderPolicySize = "logging.appenders.rolling-file.policy.size"
	LoggingRootAppenders                       = "logging.root.appenders"

	XPackSecurityAuditAppenderType       = "xpack.security.audit.appender.type"
	XPackSecurityAuditAppenderFileName   = "xpack.security.audit.appender.fileName"
	XPackSecurityAuditAppenderLayoutType = "xpack.security.audit.appender.layout.type"
	XPackSecurityAuditAppenderPolicyType = "xpack.security.audit.appender.policy.type"
	XPackSecurityAuditAppenderPolicySize = "xpack.security.audit.appender.policy.size"

	kibanaLogFilename      = "kibana.json"
	kibanaAuditLogFilename = "kibana_audit.json"
)

// MonitoringConfig returns the Kibana settings required to enable the collection of monitoring data and disk logging
func MonitoringConfig(kb kbv1.Kibana) commonv1.Config {
	cfg := commonv1.Config{}
	if monitoring.IsMetricsDefined(&kb) {
		cfg.Data = map[string]interface{}{
			MonitoringKibanaCollectionEnabled: false,
		}
	}
	if monitoring.IsLogsDefined(&kb) {
		if cfg.Data == nil {
			cfg.Data = map[string]interface{}{}
		}
		// configure the main Kibana log to be written to disk and stdout
		cfg.Data[LoggingAppendersJSONFileAppenderType] = "rolling-file"
		cfg.Data[LoggingAppendersJSONFileAppenderFilename] = filepath.Join(kibanaLogsMountPath, kibanaLogFilename)
		cfg.Data[LoggingAppendersJSONFileAppenderLayoutType] = "json"
		cfg.Data[LoggingAppendersJSONFileAppenderPolicyType] = "size-limit"
		cfg.Data[LoggingAppendersJSONFileAppenderPolicySize] = "50mb"
		cfg.Data[LoggingRootAppenders] = []string{"default", "rolling-file"}

		// configure audit logs to be written to disk so that user has just to enable audit logs to collect them
		cfg.Data[XPackSecurityAuditAppenderType] = "rolling-file"
		cfg.Data[XPackSecurityAuditAppenderFileName] = filepath.Join(kibanaLogsMountPath, kibanaAuditLogFilename)
		cfg.Data[XPackSecurityAuditAppenderLayoutType] = "json"
		cfg.Data[XPackSecurityAuditAppenderPolicyType] = "size-limit"
		cfg.Data[XPackSecurityAuditAppenderPolicySize] = "50mb"
	}
	return cfg
}
