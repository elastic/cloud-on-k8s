// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"path/filepath"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
)

var (
	MonitoringKibanaCollectionEnabled = "monitoring.kibana.collection.enabled"
	XPackSecurityAuditEnabled         = "xpack.security.audit.enabled"

	LoggingAppendersJSONFileAppenderType       = "logging.appenders.rolling-file.type"
	LoggingAppendersJSONFileAppenderFilename   = "logging.appenders.rolling-file.fileName"
	LoggingAppendersJSONFileAppenderLayoutType = "logging.appenders.rolling-file.layout.type"
	LoggingAppendersJSONFileAppenderPolicyType = "logging.appenders.rolling-file.policy.type"
	LoggingAppendersJSONFileAppenderPolicySize = "logging.appenders.rolling-file.policy.size"
	LoggingRootAppenders                       = "logging.root.appenders"
)

// MonitoringConfig returns the Kibana settings required to enable the collection of monitoring data and disk logging
func MonitoringConfig(kb kbv1.Kibana) commonv1.Config {
	cfg := commonv1.Config{}
	if IsMonitoringMetricsDefined(kb) {
		if cfg.Data == nil {
			cfg.Data = map[string]interface{}{}
		}
		cfg.Data[MonitoringKibanaCollectionEnabled] = false
	}
	if IsMonitoringLogsDefined(kb) {
		if cfg.Data == nil {
			cfg.Data = map[string]interface{}{}
		}
		cfg.Data[XPackSecurityAuditEnabled] = true
		cfg.Data[LoggingAppendersJSONFileAppenderType] = "rolling-file"
		cfg.Data[LoggingAppendersJSONFileAppenderFilename] = filepath.Join(kibanaLogsMountPath, kibanaLogFilename)
		cfg.Data[LoggingAppendersJSONFileAppenderLayoutType] = "json"
		cfg.Data[LoggingAppendersJSONFileAppenderPolicyType] = "size-limit"
		cfg.Data[LoggingAppendersJSONFileAppenderPolicySize] = "50mb"
		cfg.Data[LoggingRootAppenders] = []string{"default", "rolling-file"}
	}
	return cfg
}
