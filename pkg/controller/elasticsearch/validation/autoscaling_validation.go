// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"fmt"
	"reflect"
	"strings"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/set"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const UnexpectedVolumeClaimError = "autoscaling supports only one volume claim"

var (
	ElasticsearchMinAutoscalingVersion = version.From(7, 11, 0)

	// minMemory is the minimal amount of memory which can be set as the minimum limit in an autoscaling specification.
	minMemory = resource.MustParse("2G")

	// No minimum values are expected for CPU and Storage.
	// If provided the validation function must ensure that the value is strictly greater than 0.
	minCPU     = resource.MustParse("0")
	minStorage = resource.MustParse("0")
)

func validAutoscalingConfiguration(es esv1.Elasticsearch) field.ErrorList {
	if !es.IsAutoscalingDefined() {
		return nil
	}
	proposedVer, err := version.Parse(es.Spec.Version)
	if err != nil {
		return field.ErrorList{
			field.Invalid(field.NewPath("spec").Child("version"), es.Spec.Version, parseVersionErrMsg),
		}
	}

	var errs field.ErrorList
	if !proposedVer.IsSameOrAfter(ElasticsearchMinAutoscalingVersion) {
		errs = append(
			errs,
			field.Invalid(
				field.NewPath("metadata").Child("annotations", esv1.ElasticsearchAutoscalingSpecAnnotationName),
				es.Spec.Version,
				autoscalingVersionMsg,
			),
		)
		return errs
	}

	// Attempt to unmarshall the proposed autoscaling spec.
	autoscalingSpecification, err := es.GetAutoscalingSpecification()
	if err != nil {
		errs = append(errs, field.Invalid(
			field.NewPath("metadata").Child("annotations", esv1.ElasticsearchAutoscalingSpecAnnotationName),
			es.AutoscalingSpec(),
			err.Error(),
		))
		return errs
	}

	// Validate the autoscaling policies
	errs = append(errs, validateAutoscalingPolicies(autoscalingSpecification.AutoscalingPolicySpecs)...)
	if len(errs) > 0 {
		// We may have policies with duplicated set of roles, it may make it hard to validate further the autoscaling spec.
		return errs
	}

	// Get the list of NodeSets managed by an autoscaling policy. This requires to parse the `node.roles` field in the
	// node configuration, which may raise an error.
	autoscaledNodeSets, nodeSetConfigErr := autoscalingSpecification.GetAutoscaledNodeSets()
	if nodeSetConfigErr != nil {
		errs = append(
			errs,
			field.Invalid(
				field.NewPath("spec").Child("nodeSets").Index(nodeSetConfigErr.Index).Child("config"),
				nodeSetConfigErr.NodeSet.Config,
				fmt.Sprintf("cannot parse nodeSet configuration: %s", nodeSetConfigErr.Error()),
			),
		)
		// We stop the validation here as the named tiers are required to validate further.
		return errs
	}

	// We want to ensure that an autoscaling policy is at least managing one nodeSet.
	policiesWithNodeSets := autoscaledNodeSets.AutoscalingPolicies()
	for i, policy := range autoscalingSpecification.AutoscalingPolicySpecs {
		if !policiesWithNodeSets.Has(policy.Name) {
			// No nodeSet matches this autoscaling policy
			errs = append(
				errs,
				field.Invalid(autoscalingSpecPath(i, "roles"),
					policy.Roles,
					"roles must be used in at least one nodeSet"),
			)
		}
	}

	// Data deciders do not support multiple data paths, as a consequence only one volume claim is supported when a NodeSet
	// is managed by the autoscaling controller.
	for i, nodeSet := range autoscalingSpecification.Elasticsearch.Spec.NodeSets {
		if onlyOneVolumeClaimTemplate, _ := HasAtMostOnePersistentVolumeClaim(nodeSet); !onlyOneVolumeClaimTemplate {
			errs = append(
				errs,
				field.Invalid(
					field.NewPath("spec").Child("nodeSets").Index(i),
					nodeSet.VolumeClaimTemplates,
					UnexpectedVolumeClaimError,
				),
			)
		}
	}

	return errs
}

func validateAutoscalingPolicies(autoscalingPolicies esv1.AutoscalingPolicySpecs) field.ErrorList {
	var errs field.ErrorList
	policyNames := set.Make()
	rolesSet := make([][]string, 0, len(autoscalingPolicies))
	for i, autoscalingSpec := range autoscalingPolicies {
		// The name field is mandatory.
		if len(autoscalingSpec.Name) == 0 {
			errs = append(errs, field.Required(autoscalingSpecPath(i, "name"), "name is mandatory"))
		} else {
			if policyNames.Has(autoscalingSpec.Name) {
				errs = append(
					errs,
					field.Invalid(autoscalingSpecPath(i, "name"), autoscalingSpec.Name, "policy is duplicated"),
				)
			}
			policyNames.Add(autoscalingSpec.Name)
		}

		// Validate the set of roles managed by this autoscaling policy.
		if autoscalingSpec.Roles == nil {
			errs = append(errs, field.Required(autoscalingSpecPath(i, "roles"), "roles field is mandatory"))
		} else {
			if containsStringSlice(rolesSet, autoscalingSpec.Roles) {
				//A set of roles must be unique across all the autoscaling policies.
				errs = append(
					errs,
					field.Invalid(
						autoscalingSpecPath(i, "name"),
						strings.Join(autoscalingSpec.Roles, ","),
						"roles set is duplicated"),
				)
			} else {
				rolesSet = append(rolesSet, autoscalingSpec.Roles)
			}
		}

		// Machine learning nodes must be in a dedicated tier.
		if stringsutil.StringInSlice(esv1.MLRole, autoscalingSpec.Roles) && len(autoscalingSpec.Roles) > 1 {
			errs = append(
				errs,
				field.Invalid(
					autoscalingSpecPath(i, "name"), strings.Join(autoscalingSpec.Roles, ","),
					"ML nodes must be in a dedicated autoscaling policy"),
			)
		}

		if !(autoscalingSpec.NodeCount.Min >= 0) {
			errs = append(
				errs,
				field.Invalid(
					autoscalingSpecPath(i, "resources", "nodeCount", "min"),
					autoscalingSpec.NodeCount.Min,
					"min count must be equal or greater than 0",
				),
			)
		}

		if !(autoscalingSpec.NodeCount.Max > 0) {
			errs = append(
				errs,
				field.Invalid(
					autoscalingSpecPath(i, "resources", "nodeCount", "max"),
					autoscalingSpec.NodeCount.Max,
					"max count must be greater than 0"),
			)
		}

		if !(autoscalingSpec.NodeCount.Max >= autoscalingSpec.NodeCount.Min) {
			errs = append(
				errs,
				field.Invalid(autoscalingSpecPath(i, "resources", "nodeCount", "max"),
					autoscalingSpec.NodeCount.Max,
					"max node count must be an integer greater or equal than the min node count"),
			)
		}

		// Validate CPU
		errs = validateQuantities(errs, autoscalingSpec.CPU, i, "cpu", minCPU)

		// Validate Memory
		errs = validateQuantities(errs, autoscalingSpec.Memory, i, "memory", minMemory)

		// Validate storage
		errs = validateQuantities(errs, autoscalingSpec.Storage, i, "storage", minStorage)
	}
	return errs
}

// autoscalingSpecPath helps to compute the path used in validation error fields.
func autoscalingSpecPath(index int, child string, moreChildren ...string) *field.Path {
	return field.NewPath("metadata").
		Child("annotations", `"`+esv1.ElasticsearchAutoscalingSpecAnnotationName+`"`).
		Index(index).
		Child(child, moreChildren...)
}

// validateQuantities ensures that a quantity range is valid.
func validateQuantities(
	errs field.ErrorList,
	quantityRange *esv1.QuantityRange,
	index int,
	resource string,
	minQuantity resource.Quantity,
) field.ErrorList {
	var quantityErrs field.ErrorList
	if quantityRange == nil {
		return errs
	}

	if !minQuantity.IsZero() && quantityRange.Min.Cmp(minQuantity) < 0 {
		quantityErrs = append(
			quantityErrs,
			field.Required(
				autoscalingSpecPath(index, "minAllowed", resource),
				fmt.Sprintf("min quantity must be greater than %s", minQuantity.String())),
		)
	}

	// A quantity must always be greater than 0.
	if !(quantityRange.Min.Value() > 0) {
		quantityErrs = append(
			quantityErrs,
			field.Required(
				autoscalingSpecPath(index, "minAllowed", resource),
				"min quantity must be greater than 0"),
		)
	}

	if quantityRange.Min.Cmp(quantityRange.Max) > 0 {
		quantityErrs = append(
			quantityErrs,
			field.Invalid(
				autoscalingSpecPath(index, "maxAllowed", resource), quantityRange.Max.String(),
				"max quantity must be greater or equal than min quantity"),
		)
	}
	return append(errs, quantityErrs...)
}

// containsStringSlice returns true if a slice of strings is included in a slice of slices of strings.
func containsStringSlice(slices [][]string, slice []string) bool {
	set1 := set.Make(slice...)
	for _, candidate := range slices {
		set2 := set.Make(candidate...)
		if len(set1) != len(set2) {
			continue
		}
		if reflect.DeepEqual(set1, set2) {
			return true
		}
	}
	return false
}

// HasAtMostOnePersistentVolumeClaim returns true if the NodeSet has only one volume claim template. It also returns
// the name of the volume claim template in that case.
func HasAtMostOnePersistentVolumeClaim(nodeSet esv1.NodeSet) (bool, string) {
	//volumeClaimTemplates := len(nodeSet.VolumeClaimTemplates)
	switch len(nodeSet.VolumeClaimTemplates) {
	case 0:
		return true, ""
	case 1:
		return true, nodeSet.VolumeClaimTemplates[0].Name
	}
	return false, ""
}
