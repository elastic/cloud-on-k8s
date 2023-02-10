// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

var (
	defaultChecks = []func(*Logstash) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkSingleConfigSource,
	}

	updateChecks = []func(old, curr *Logstash) field.ErrorList{
		checkNoDowngrade,
	}
)

func checkNoUnknownFields(a *Logstash) field.ErrorList {
	return commonv1.NoUnknownFields(a, a.ObjectMeta)
}

func checkNameLength(a *Logstash) field.ErrorList {
	return commonv1.CheckNameLength(a)
}

func checkSupportedVersion(a *Logstash) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(a.Spec.Version, version.SupportedLogstashVersions)
}

func checkNoDowngrade(prev, curr *Logstash) field.ErrorList {
	if commonv1.IsConfiguredToAllowDowngrades(curr) {
		return nil
	}
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}

func checkSingleConfigSource(a *Logstash) field.ErrorList {
	if a.Spec.Config != nil && a.Spec.ConfigRef != nil {
		msg := "Specify at most one of [`config`, `configRef`], not both"
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("config"), msg),
			field.Forbidden(field.NewPath("spec").Child("configRef"), msg),
		}
	}

	return nil
}