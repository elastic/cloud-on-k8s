// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var (
	defaultChecks = []func(*Agent) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkAtMostOneDeploymentOption,
		checkAtMostOneDefaultESRef,
		checkESRefsNamed,
		checkSingleConfigSource,
		checkSpec,
	}

	updateChecks = []func(old, curr *Agent) field.ErrorList{
		checkNoDowngrade,
	}
)

func checkNoUnknownFields(b *Agent) field.ErrorList {
	return commonv1.NoUnknownFields(b, b.ObjectMeta)
}

func checkNameLength(ent *Agent) field.ErrorList {
	return commonv1.CheckNameLength(ent)
}

func checkSupportedVersion(b *Agent) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(b.Spec.Version, version.SupportedAgentVersions)
}

func checkAtMostOneDeploymentOption(b *Agent) field.ErrorList {
	if b.Spec.DaemonSet != nil && b.Spec.Deployment != nil {
		msg := "Specify either daemonSet or deployment, not both"
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("daemonSet"), msg),
			field.Forbidden(field.NewPath("spec").Child("deployment"), msg),
		}
	}

	return nil
}

func checkAtMostOneDefaultESRef(b *Agent) field.ErrorList {
	var found int
	for _, o := range b.Spec.ElasticsearchRefs {
		if o.OutputName == "default" {
			found++
		}
	}
	if found > 1 {
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("elasticsearchRefs"), "only one elasticsearchRef may have the outputName 'default'"),
		}
	}
	return nil
}

func checkESRefsNamed(b *Agent) field.ErrorList {
	if len(b.Spec.ElasticsearchRefs) <= 1 {
		// a single output does not need to be named
		return nil
	}
	var notNamed []string
	for _, o := range b.Spec.ElasticsearchRefs {
		if o.OutputName == "" {
			notNamed = append(notNamed, o.NamespacedName().String())
		}
	}
	if len(notNamed) > 0 {
		msg := fmt.Sprintf("when declaring mulitiple refs all have to be named, missing outputName on %v", notNamed)
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("elasticsearchRefs"), msg),
		}
	}
	return nil
}

func checkNoDowngrade(prev, curr *Agent) field.ErrorList {
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}

func checkSingleConfigSource(b *Agent) field.ErrorList {
	if b.Spec.Config != nil && b.Spec.ConfigRef != nil {
		msg := "Specify at most one of [`config`, `configRef`], not both"
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("config"), msg),
			field.Forbidden(field.NewPath("spec").Child("configRef"), msg),
		}
	}

	return nil
}

func checkSpec(b *Agent) field.ErrorList {
	if (b.Spec.DaemonSet == nil && b.Spec.Deployment == nil) || (b.Spec.DaemonSet != nil && b.Spec.Deployment != nil) {
		return field.ErrorList{
			field.Invalid(field.NewPath("spec"), b.Spec, "either daemonset or deployment must be specified"),
		}
	}
	return nil
}
