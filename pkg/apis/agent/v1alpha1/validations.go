// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"fmt"
	"reflect"

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
		checkEmptyConfigForFleetMode,
		checkFleetServerOnlyInFleetMode,
		checkHttpConfigOnlyForFleetServer,
		checkFleetServerOrFleetServerRef,
		checkReferenceSetForMode,
	}

	updateChecks = []func(old, curr *Agent) field.ErrorList{
		checkNoDowngrade,
	}
)

func checkNoUnknownFields(a *Agent) field.ErrorList {
	return commonv1.NoUnknownFields(a, a.ObjectMeta)
}

func checkNameLength(a *Agent) field.ErrorList {
	return commonv1.CheckNameLength(a)
}

func checkSupportedVersion(a *Agent) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(a.Spec.Version, version.SupportedAgentVersions)
}

func checkAtMostOneDeploymentOption(a *Agent) field.ErrorList {
	if a.Spec.DaemonSet != nil && a.Spec.Deployment != nil {
		msg := "Specify either daemonSet or deployment, not both"
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("daemonSet"), msg),
			field.Forbidden(field.NewPath("spec").Child("deployment"), msg),
		}
	}

	return nil
}

func checkAtMostOneDefaultESRef(a *Agent) field.ErrorList {
	var found int
	for _, o := range a.Spec.ElasticsearchRefs {
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

func checkESRefsNamed(a *Agent) field.ErrorList {
	if len(a.Spec.ElasticsearchRefs) <= 1 {
		// a single output does not need to be named
		return nil
	}
	var notNamed []string
	for _, o := range a.Spec.ElasticsearchRefs {
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

func checkSingleConfigSource(a *Agent) field.ErrorList {
	if a.Spec.Config != nil && a.Spec.ConfigRef != nil {
		msg := "Specify at most one of [`config`, `configRef`], not both"
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("config"), msg),
			field.Forbidden(field.NewPath("spec").Child("configRef"), msg),
		}
	}

	return nil
}

func checkSpec(a *Agent) field.ErrorList {
	if (a.Spec.DaemonSet == nil && a.Spec.Deployment == nil) || (a.Spec.DaemonSet != nil && a.Spec.Deployment != nil) {
		return field.ErrorList{
			field.Invalid(field.NewPath("spec"), a.Spec, "either daemonset or deployment must be specified"),
		}
	}
	return nil
}

func checkEmptyConfigForFleetMode(a *Agent) field.ErrorList {
	var errors field.ErrorList
	if a.Spec.Mode == AgentFleetMode {
		if a.Spec.Config != nil {
			errors = append(errors, field.Invalid(
				field.NewPath("spec").Child("config"),
				a.Spec.Config,
				"remove config, it can't be set in the fleet mode",
			))
		}

		if a.Spec.ConfigRef != nil {
			errors = append(errors, field.Invalid(
				field.NewPath("spec").Child("configRef"),
				a.Spec.ConfigRef,
				"remove configRef, it can't be set in the fleet mode",
			))
		}
	}

	return errors
}

func checkFleetServerOnlyInFleetMode(a *Agent) field.ErrorList {
	if a.Spec.Mode == "" || a.Spec.Mode == AgentStandaloneMode {
		if a.Spec.EnableFleetServer {
			return field.ErrorList{field.Invalid(
				field.NewPath("spec").Child("enableFleetServer"),
				a.Spec.EnableFleetServer,
				"disable Fleet Server, it can't be enabled in the standalone mode",
			)}
		}
	}

	return nil
}

func checkFleetServerOrFleetServerRef(a *Agent) field.ErrorList {
	if a.Spec.EnableFleetServer && a.Spec.FleetServerRef.IsDefined() {
		return field.ErrorList{
			field.Invalid(
				field.NewPath("spec"),
				a.Spec,
				"enable Fleet Server or specify Fleet Server reference, not both",
			),
		}
	}
	return nil
}

func checkHttpConfigOnlyForFleetServer(a *Agent) field.ErrorList {
	if !a.Spec.EnableFleetServer && !reflect.DeepEqual(a.Spec.HTTP, commonv1.HTTPConfig{}) {
		return field.ErrorList{
			field.Invalid(
				field.NewPath("spec").Child("http"),
				a.Spec.HTTP,
				"don't specify http configuration, it can't be set when Fleet Server is not enabled",
			),
		}
	}
	return nil
}

func checkReferenceSetForMode(a *Agent) field.ErrorList {
	var errors field.ErrorList
	if a.Spec.Mode == "" || a.Spec.Mode == AgentStandaloneMode {
		if a.Spec.FleetServerRef.IsDefined() {
			errors = append(errors, field.Invalid(
				field.NewPath("spec").Child("fleetServerRef"),
				a.Spec.FleetServerRef,
				"don't specify Fleet Server reference, it can't be set in the standalone mode",
			))
		}

		if a.Spec.KibanaRef.IsDefined() {
			errors = append(errors, field.Invalid(
				field.NewPath("spec").Child("kibanaRef"),
				a.Spec.KibanaRef,
				"don't specify Kibana reference, it can't be set in the standalone mode",
			))
		}
	} else if a.Spec.Mode == AgentFleetMode {
		if !a.Spec.EnableFleetServer && len(a.Spec.ElasticsearchRefs) > 0 {
			errors = append(errors, field.Invalid(
				field.NewPath("spec").Child("enableFleetServer"),
				a.Spec.EnableFleetServer,
				"remove Elasticsearch reference, it can't be enabled in the fleet mode when Fleet Server is not enabled as well",
			))
		}
	}

	return errors
}
