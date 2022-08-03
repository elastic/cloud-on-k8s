// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/validations"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

var (
	defaultChecks = []func(*Beat) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkAtMostOneDeploymentOption,
		checkImageIfTypeUnknown,
		checkBeatType,
		checkSingleConfigSource,
		checkSpec,
		checkAssociations,
		checkMonitoring,
	}

	updateChecks = []func(old, curr *Beat) field.ErrorList{
		checkNoDowngrade,
	}

	typeRegex = regexp.MustCompile("^[a-zA-Z0-9-]+$")
	// minStackVersion is the minimum Stack version to enable Stack Monitoring on an Elastic Beats.
	// Technically, Beats internal monitoring existed prior to 7.2.0, but it's configuration format
	// was slightly different.
	minStackVersion = version.MustParse("7.2.0")
)

func checkNoUnknownFields(b *Beat) field.ErrorList {
	return commonv1.NoUnknownFields(b, b.ObjectMeta)
}

func checkNameLength(ent *Beat) field.ErrorList {
	return commonv1.CheckNameLength(ent)
}

func checkSupportedVersion(b *Beat) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(b.Spec.Version, version.SupportedBeatVersions)
}

func checkAtMostOneDeploymentOption(b *Beat) field.ErrorList {
	if b.Spec.DaemonSet != nil && b.Spec.Deployment != nil {
		msg := "Specify either daemonSet or deployment, not both"
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("daemonSet"), msg),
			field.Forbidden(field.NewPath("spec").Child("deployment"), msg),
		}
	}

	return nil
}

func checkImageIfTypeUnknown(b *Beat) field.ErrorList {
	if _, ok := KnownTypes[b.Spec.Type]; !ok && b.Spec.Image == "" {
		return field.ErrorList{
			field.Required(
				field.NewPath("spec").Child("image"),
				"Image is required if Beat type is not one of [filebeat, metricbeat, heartbeat, auditbeat, journalbeat, packetbeat]"),
		}
	}
	return nil
}

func checkBeatType(b *Beat) field.ErrorList {
	if !typeRegex.MatchString(b.Spec.Type) {
		return field.ErrorList{
			field.Invalid(
				field.NewPath("spec").Child("type"),
				b.Spec.Type,
				"Beat Type has to match ^[a-zA-Z0-9-]+$"),
		}
	}

	return nil
}

func checkNoDowngrade(prev, curr *Beat) field.ErrorList {
	if commonv1.IsConfiguredToAllowDowngrades(curr) {
		return nil
	}
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}

func checkSingleConfigSource(b *Beat) field.ErrorList {
	if b.Spec.Config != nil && b.Spec.ConfigRef != nil {
		msg := "Specify at most one of [`config`, `configRef`], not both"
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("config"), msg),
			field.Forbidden(field.NewPath("spec").Child("configRef"), msg),
		}
	}

	return nil
}

func checkSpec(b *Beat) field.ErrorList {
	if (b.Spec.DaemonSet == nil && b.Spec.Deployment == nil) || (b.Spec.DaemonSet != nil && b.Spec.Deployment != nil) {
		return field.ErrorList{
			field.Invalid(field.NewPath("spec"), b.Spec, "either daemonset or deployment must be specified"),
		}
	}
	return nil
}

func checkAssociations(b *Beat) field.ErrorList {
	err1 := commonv1.CheckAssociationRefs(field.NewPath("spec").Child("elasticsearchRef"), b.Spec.ElasticsearchRef)
	err2 := commonv1.CheckAssociationRefs(field.NewPath("spec").Child("kibanaRef"), b.Spec.KibanaRef)
	return append(err1, err2...)
}

func checkMonitoring(b *Beat) field.ErrorList {
	var errs field.ErrorList
	if len(b.GetMonitoringMetricsRefs()) == 0 && len(b.GetMonitoringLogsRefs()) == 0 {
		return nil
	}
	if err := isStackSupportedVersion(b.Spec.Version); err != nil {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("version"), b.Spec.Version, err.Error()))
	}
	refs := b.GetMonitoringMetricsRefs()
	refsDefined := monitoring.AreEsRefsDefined(refs)
	if len(refs) > 0 && !refsDefined {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("monitoring").Child("metrics").Child("elasticsearchRefs"),
			refs, validations.InvalidBeatsElasticsearchRefForStackMonitoringMsg))
	}
	if refsDefined && len(refs) != 1 {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("monitoring").Child("metrics").Child("elasticsearchRefs"),
			refs, validations.InvalidElasticsearchRefsMsg))
	}
	refs = b.GetMonitoringLogsRefs()
	refsDefined = monitoring.AreEsRefsDefined(refs)
	if len(refs) > 0 && !refsDefined {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("monitoring").Child("logs").Child("elasticsearchRefs"),
			refs, validations.InvalidBeatsElasticsearchRefForStackMonitoringMsg))
	}
	if refsDefined && len(refs) != 1 {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("monitoring").Child("logs").Child("elasticsearchRefs"),
			refs, validations.InvalidElasticsearchRefsMsg))
	}
	return errs
}

func isStackSupportedVersion(v string) error {
	ver, err := version.Parse(v)
	if err != nil {
		return err
	}
	if ver.LT(minStackVersion) {
		return fmt.Errorf(validations.UnsupportedVersionMsg, minStackVersion)
	}
	return nil
}
