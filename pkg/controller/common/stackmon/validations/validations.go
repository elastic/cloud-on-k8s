// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validations

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	unsupportedVersionForStackMonitoringMsg    = "Unsupported version for Stack Monitoring. Required >= %s."
	invalidStackMonitoringElasticsearchRefsMsg = "Only one Elasticsearch reference is supported for %s Stack Monitoring"
)

var (
	// MinStackVersion is the minimum Stack version to enable Stack Monitoring on an Elastic Stack application..
	// This requirement comes from the fact that we configure Elasticsearch to write logs to disk for Filebeat
	// via the env var ES_LOG_STYLE available from this version.
	MinStackVersion = version.MustParse("7.14.0-SNAPSHOT")
)

// Validate validates that the resource version is supported for Stack Monitoring and that there is exactly one
// Elasticsearch reference defined to send monitoring data when Stack Monitoring is defined
func Validate(resource monitoring.HasMonitoring, version string) field.ErrorList {
	var errs field.ErrorList
	if monitoring.IsMonitoringDefined(resource) {
		err := IsSupportedVersion(version)
		if err != nil {
			errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), version,
				fmt.Sprintf(unsupportedVersionForStackMonitoringMsg, MinStackVersion)))
		}
	}
	refs := resource.GetMonitoringMetricsRefs()
	if monitoring.AreEsRefsDefined(refs) && len(refs) != 1 {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("monitoring").Child("metrics").Child("elasticsearchRefs"),
			refs, fmt.Sprintf(invalidStackMonitoringElasticsearchRefsMsg, "Metrics")))
	}
	refs = resource.GetMonitoringLogsRefs()
	if monitoring.AreEsRefsDefined(refs) && len(refs) != 1 {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("monitoring").Child("logs").Child("elasticsearchRefs"),
			refs, fmt.Sprintf(invalidStackMonitoringElasticsearchRefsMsg, "Logs")))
	}
	return errs
}

// IsSupportedVersion returns true if the resource version is supported for Stack Monitoring, else returns false
func IsSupportedVersion(v string) error {
	ver, err := version.Parse(v)
	if err != nil {
		return err
	}
	if ver.LT(MinStackVersion) {
		return fmt.Errorf("unsupported version for Stack Monitoring: required >= %s", MinStackVersion)
	}
	return nil
}
