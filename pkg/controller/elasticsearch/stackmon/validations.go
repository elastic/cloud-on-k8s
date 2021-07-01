// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"k8s.io/apimachinery/pkg/util/validation/field"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
)

const (
	unsupportedVersionForStackMonitoringMsg    = "Unsupported version for Stack Monitoring. Required >= %s."
	invalidStackMonitoringElasticsearchRefsMsg = "Only one Elasticsearch reference is supported for %s Stack Monitoring"
)

var (
	// Minimum Stack version to enable Stack Monitoring.
	// This requirement comes from the fact that we configure Elasticsearch to write logs to disk for Filebeat
	// via the env var ES_LOG_STYLE available from this version.
	MinStackVersion = version.MustParse("7.14.0")
)

// Validate validates that the Elasticsearch version is supported for Stack Monitoring and that there is exactly one
// Elasticsearch reference defined when Stack Monitoring is defined
func Validate(es esv1.Elasticsearch) field.ErrorList {
	if isMonitoringDefined(es) {
		err := IsSupportedVersion(es.Spec.Version)
		if err != nil {
			return field.ErrorList{field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version,
				fmt.Sprintf(unsupportedVersionForStackMonitoringMsg, MinStackVersion))}
		}
	}
	if IsMonitoringMetricsDefined(es) && len(es.Spec.Monitoring.Metrics.ElasticsearchRefs) != 1 {
		return field.ErrorList{field.Invalid(field.NewPath("spec").Child("monitoring").Child("metrics").Child("elasticsearchRefs"),
			es.Spec.Monitoring.Metrics.ElasticsearchRefs,
			fmt.Sprintf(invalidStackMonitoringElasticsearchRefsMsg, "Metrics"))}
	}
	if IsMonitoringLogsDefined(es) && len(es.Spec.Monitoring.Logs.ElasticsearchRefs) != 1 {
		return field.ErrorList{field.Invalid(field.NewPath("spec").Child("monitoring").Child("logs").Child("elasticsearchRefs"),
			es.Spec.Monitoring.Logs.ElasticsearchRefs,
			fmt.Sprintf(invalidStackMonitoringElasticsearchRefsMsg, "Logs"))}
	}
	return field.ErrorList{}
}

// IsSupportedVersion returns true if the Elasticsearch version is supported for Stack Monitoring, else returns false
func IsSupportedVersion(esVersion string) error {
	ver, err := version.Parse(esVersion)
	if err != nil {
		return err
	}
	if ver.LT(MinStackVersion) {
		return fmt.Errorf("unsupported version for Stack Monitoring: required >= %s", MinStackVersion)
	}
	return nil
}
