// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validations

import (
	"fmt"

	"github.com/blang/semver/v4"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

const (
	UnsupportedVersionMsg       = "Unsupported version for Stack Monitoring. Required >= %s."
	InvalidElasticsearchRefsMsg = "Only one Elasticsearch reference is supported for %s Stack Monitoring"

	InvalidKibanaElasticsearchRefForStackMonitoringMsg = "Kibana must be associated to an Elasticsearch cluster through elasticsearchRef in order to enable monitoring metrics features"
	InvalidBeatsElasticsearchRefForStackMonitoringMsg  = "Beats must be associated to an Elasticsearch cluster through elasticsearchRef in order to enable monitoring metrics features"
)

var (
	// MinStackVersion is the minimum Stack version to enable Stack Monitoring on an Elastic Stack application..
	// This requirement comes from the fact that we configure Elasticsearch to write logs to disk for Filebeat
	// via the env var ES_LOG_STYLE available from this version.
	MinStackVersion = version.MustParse("7.14.0-SNAPSHOT")
)

// Validate validates that the resource version is supported for Stack Monitoring and that there is exactly one
// Elasticsearch reference defined to send monitoring data when Stack Monitoring is defined
func Validate(resource monitoring.HasMonitoring, version string, minVersion version.Version) field.ErrorList {
	var errs field.ErrorList
	if monitoring.IsDefined(resource) {
		err := IsSupportedVersion(version, minVersion)
		if err != nil {
			finalMinStackVersion, _ := semver.FinalizeVersion(minVersion.String()) // discards prerelease suffix
			errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), version,
				fmt.Sprintf(UnsupportedVersionMsg, finalMinStackVersion)))
		}
	}
	refs := resource.GetMonitoringMetricsRefs()
	if monitoring.AreEsRefsDefined(refs) && len(refs) != 1 {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("monitoring").Child("metrics").Child("elasticsearchRefs"),
			refs, fmt.Sprintf(InvalidElasticsearchRefsMsg, "Metrics")))
	}
	refs = resource.GetMonitoringLogsRefs()
	if monitoring.AreEsRefsDefined(refs) && len(refs) != 1 {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("monitoring").Child("logs").Child("elasticsearchRefs"),
			refs, fmt.Sprintf(InvalidElasticsearchRefsMsg, "Logs")))
	}
	return errs
}

// IsSupportedVersion returns error if the resource version is not supported for Stack Monitoring
func IsSupportedVersion(v string, minVersion version.Version) error {
	ver, err := version.Parse(v)
	if err != nil {
		return err
	}
	if ver.LT(minVersion) {
		return fmt.Errorf("unsupported version for Stack Monitoring: required >= %s", minVersion)
	}
	return nil
}
