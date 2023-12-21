// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"fmt"
	"reflect"
	"strings"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

var (
	defaultChecks = []func(*Agent) field.ErrorList{
		checkPolicyID,
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
		checkHTTPConfigOnlyForFleetServer,
		checkFleetServerOrFleetServerRef,
		checkReferenceSetForMode,
		checkSingleESRefInFleetMode,
		checkAssociations,
	}

	updateChecks = []func(old, curr *Agent) field.ErrorList{
		checkNoDowngrade,
		checkPVCchanges,
	}
)

const (
	pvcImmutableMsg = "Volume claim templates cannot be modified"
)

func checkNoUnknownFields(a *Agent) field.ErrorList {
	return commonv1.NoUnknownFields(a, a.ObjectMeta)
}

func checkNameLength(a *Agent) field.ErrorList {
	return commonv1.CheckNameLength(a)
}

func checkSupportedVersion(a *Agent) field.ErrorList {
	if a.Spec.FleetModeEnabled() {
		return commonv1.CheckSupportedStackVersion(a.Spec.Version, version.SupportedFleetModeAgentVersions)
	}

	return commonv1.CheckSupportedStackVersion(a.Spec.Version, version.SupportedAgentVersions)
}

func checkPolicyID(a *Agent) field.ErrorList {
	v, err := commonv1.ParseVersion(a.Spec.Version)
	if err != nil {
		return err
	}
	if v.GTE(MandatoryPolicyIDVersion) && len(a.Spec.PolicyID) == 0 {
		msg := "Agent policyID is mandatory"
		return field.ErrorList{
			field.Required(field.NewPath("spec").Child("policyID"), msg),
		}
	}
	return nil
}

func checkAtMostOneDeploymentOption(a *Agent) field.ErrorList {
	var enabledSpecsNames []string

	if a.Spec.DaemonSet != nil {
		enabledSpecsNames = append(enabledSpecsNames, "daemonSet")
	}
	if a.Spec.Deployment != nil {
		enabledSpecsNames = append(enabledSpecsNames, "deployment")
	}
	if a.Spec.StatefulSet != nil {
		enabledSpecsNames = append(enabledSpecsNames, "statefulSet")
	}

	if enabledSpecsLen := len(enabledSpecsNames); enabledSpecsLen > 1 {
		msg := fmt.Sprintf("Specify at most one of [%s]", strings.Join(enabledSpecsNames, ", "))
		errList := make(field.ErrorList, enabledSpecsLen)
		for index, specName := range enabledSpecsNames {
			errList[index] = field.Forbidden(field.NewPath("spec").Child(specName), msg)
		}
		return errList
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
		msg := fmt.Sprintf("when declaring multiple refs all have to be named, missing outputName on %v", notNamed)
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("elasticsearchRefs"), msg),
		}
	}
	return nil
}

func checkNoDowngrade(prev, curr *Agent) field.ErrorList {
	if commonv1.IsConfiguredToAllowDowngrades(curr) {
		return nil
	}
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}

// checkPVCchanges ensures no PVCs are changed, as volume claim templates are immutable in StatefulSets.
func checkPVCchanges(current, proposed *Agent) field.ErrorList {
	var errs field.ErrorList
	if current == nil || proposed == nil {
		return errs
	}

	// need to check if current and proposed are both statefulsets
	if current.Spec.StatefulSet == nil || proposed.Spec.StatefulSet == nil {
		return errs
	}

	// checking semantic equality here allows providing PVC storage size with different units (eg. 1Ti vs. 1024Gi).
	if !apiequality.Semantic.DeepEqual(current.Spec.StatefulSet.VolumeClaimTemplates, proposed.Spec.StatefulSet.VolumeClaimTemplates) {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("statefulSet.").Child("volumeClaimTemplates"),
			proposed.Spec.StatefulSet.VolumeClaimTemplates, pvcImmutableMsg))
	}

	return errs
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
	enabledSpecs := 0
	if a.Spec.DaemonSet != nil {
		enabledSpecs++
	}
	if a.Spec.Deployment != nil {
		enabledSpecs++
	}
	if a.Spec.StatefulSet != nil {
		enabledSpecs++
	}

	if enabledSpecs != 1 {
		return field.ErrorList{
			field.Invalid(field.NewPath("spec"), a.Spec, "either daemonSet or deployment or statefulSet must be specified"),
		}
	}
	return nil
}

func checkEmptyConfigForFleetMode(a *Agent) field.ErrorList {
	var errors field.ErrorList
	if a.Spec.FleetModeEnabled() {
		if a.Spec.Config != nil {
			errors = append(errors, field.Invalid(
				field.NewPath("spec").Child("config"),
				a.Spec.Config,
				"remove config, it can't be set in fleet mode",
			))
		}

		if a.Spec.ConfigRef != nil {
			errors = append(errors, field.Invalid(
				field.NewPath("spec").Child("configRef"),
				a.Spec.ConfigRef,
				"remove configRef, it can't be set in fleet mode",
			))
		}
	}

	return errors
}

func checkFleetServerOnlyInFleetMode(a *Agent) field.ErrorList {
	if a.Spec.StandaloneModeEnabled() && a.Spec.FleetServerEnabled {
		return field.ErrorList{field.Invalid(
			field.NewPath("spec").Child("fleetServerEnabled"),
			a.Spec.FleetServerEnabled,
			"disable Fleet Server, it can't be enabled in standalone mode",
		)}
	}
	return nil
}

func checkFleetServerOrFleetServerRef(a *Agent) field.ErrorList {
	if a.Spec.FleetServerEnabled && a.Spec.FleetServerRef.IsDefined() {
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

func checkHTTPConfigOnlyForFleetServer(a *Agent) field.ErrorList {
	if !a.Spec.FleetServerEnabled && !reflect.DeepEqual(a.Spec.HTTP, commonv1.HTTPConfig{}) {
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
	if a.Spec.StandaloneModeEnabled() {
		if a.Spec.FleetServerRef.IsDefined() {
			errors = append(errors, field.Invalid(
				field.NewPath("spec").Child("fleetServerRef"),
				a.Spec.FleetServerRef,
				"don't specify Fleet Server reference, it can't be set in standalone mode",
			))
		}

		if a.Spec.KibanaRef.IsDefined() {
			errors = append(errors, field.Invalid(
				field.NewPath("spec").Child("kibanaRef"),
				a.Spec.KibanaRef,
				"don't specify Kibana reference, it can't be set in standalone mode",
			))
		}
	} else if a.Spec.FleetModeEnabled() {
		if !a.Spec.FleetServerEnabled && len(a.Spec.ElasticsearchRefs) > 0 {
			errors = append(errors, field.Invalid(
				field.NewPath("spec").Child("fleetServerEnabled"),
				a.Spec.FleetServerEnabled,
				"remove Elasticsearch reference, it can't be enabled in fleet mode when Fleet Server is not enabled as well",
			))
		}
	}

	return errors
}

func checkSingleESRefInFleetMode(a *Agent) field.ErrorList {
	if a.Spec.FleetModeEnabled() && len(a.Spec.ElasticsearchRefs) > 1 {
		return field.ErrorList{
			field.Invalid(
				field.NewPath("spec").Child("elasticsearchRefs"),
				a.Spec.HTTP,
				"don't specify more than one Elasticsearch reference, this is not supported in fleet mode",
			),
		}
	}
	return nil
}

func checkAssociations(a *Agent) field.ErrorList {
	err1 := commonv1.CheckAssociationRefs(field.NewPath("spec").Child("elasticsearchRefs"), a.ElasticsearchRefs()...)
	err2 := commonv1.CheckAssociationRefs(field.NewPath("spec").Child("kibanaRef"), a.Spec.KibanaRef)
	err3 := commonv1.CheckAssociationRefs(field.NewPath("spec").Child("fleetServerRef"), a.Spec.FleetServerRef)
	return append(append(err1, err2...), err3...)
}
