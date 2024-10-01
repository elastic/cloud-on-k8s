// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoscaling

import (
	"fmt"

	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/stringsutil"
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

func ValidateAutoscalingSpecification(
	autoscalingSpecPath SpecPathBuilder,
	autoscalingPolicySpecs v1alpha1.AutoscalingPolicySpecs,
	es esv1.Elasticsearch,
	v version.Version,
) field.ErrorList {
	var errs field.ErrorList
	// Get the list of NodeSets managed by an autoscaling policy. This requires to parse the `node.roles` field in the
	// node configuration, which may raise an error.
	autoscaledNodeSets, nodeSetConfigErr := es.GetAutoscaledNodeSets(v, autoscalingPolicySpecs)
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
	for i, policy := range autoscalingPolicySpecs {
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
	for i, nodeSet := range es.Spec.NodeSets {
		if onlyOneVolumeClaimTemplate, _ := HasAtMostOnePersistentVolumeClaim(nodeSet); !onlyOneVolumeClaimTemplate {
			errs = append(
				errs,
				field.Invalid(
					field.NewPath("Elasticsearch").Child("spec").Child("nodeSets").Index(i),
					volumeClaimTemplatesNames(nodeSet.VolumeClaimTemplates),
					UnexpectedVolumeClaimError,
				),
			)
		}
	}

	return errs
}

func volumeClaimTemplatesNames(claims []corev1.PersistentVolumeClaim) []string {
	names := make([]string, len(claims))
	for i := range claims {
		names[i] = claims[i].Name
	}
	return names
}

type SpecPathBuilder func(index int, child string, moreChildren ...string) *field.Path

func ValidateAutoscalingPolicies(
	autoscalingSpecPath SpecPathBuilder,
	autoscalingPolicies v1alpha1.AutoscalingPolicySpecs,
) field.ErrorList {
	var errs field.ErrorList
	policyNames := set.Make()
	mlPolicyCount := 0
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
		if len(autoscalingSpec.Roles) == 0 {
			errs = append(errs, field.Required(autoscalingSpecPath(i, "roles"), "roles field is mandatory and must not be empty"))
		} else {
			if containsStringSlice(rolesSet, autoscalingSpec.Roles) {
				// A set of roles must be unique across all the autoscaling policies.
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

		if stringsutil.StringInSlice(string(esv1.MLRole), autoscalingSpec.Roles) {
			mlPolicyCount++
		}

		if mlPolicyCount > 1 {
			errs = append(
				errs,
				field.Invalid(
					autoscalingSpecPath(i, "name"), strings.Join(autoscalingSpec.Roles, ","),
					"ML nodes must be in a dedicated NodeSet",
				),
			)
		}

		// Machine learning nodes must be in a dedicated tier.
		if stringsutil.StringInSlice(string(esv1.MLRole), autoscalingSpec.Roles) && len(ignoreRemoteClusterClientRole(autoscalingSpec.Roles)) > 1 {
			errs = append(
				errs,
				field.Invalid(
					autoscalingSpecPath(i, "name"), strings.Join(autoscalingSpec.Roles, ","),
					"ML nodes must be in a dedicated autoscaling policy"),
			)
		}

		if !(autoscalingSpec.NodeCountRange.Min >= 0) {
			errs = append(
				errs,
				field.Invalid(
					autoscalingSpecPath(i, "resources", "nodeCount", "min"),
					autoscalingSpec.NodeCountRange.Min,
					"min count must be equal or greater than 0",
				),
			)
		}

		if !(autoscalingSpec.NodeCountRange.Max > 0) {
			errs = append(
				errs,
				field.Invalid(
					autoscalingSpecPath(i, "resources", "nodeCount", "max"),
					autoscalingSpec.NodeCountRange.Max,
					"max count must be greater than 0"),
			)
		}

		if !(autoscalingSpec.NodeCountRange.Max >= autoscalingSpec.NodeCountRange.Min) {
			errs = append(
				errs,
				field.Invalid(autoscalingSpecPath(i, "resources", "nodeCount", "max"),
					autoscalingSpec.NodeCountRange.Max,
					"max node count must be an integer greater or equal than the min node count"),
			)
		}

		// Validate CPU
		errs = validateQuantities(errs, autoscalingSpecPath, autoscalingSpec.CPURange, i, "cpu", minCPU)

		// Validate Memory
		errs = validateQuantities(errs, autoscalingSpecPath, autoscalingSpec.MemoryRange, i, "memory", minMemory)

		// Validate storage
		errs = validateQuantities(errs, autoscalingSpecPath, autoscalingSpec.StorageRange, i, "storage", minStorage)
	}

	return errs
}

// ignoreRemoteClusterClientRole will ignore the 'remote_cluster_client' role in a given slice of roles.
func ignoreRemoteClusterClientRole(roles []string) []string {
	var updatedRoles []string
	for _, role := range roles {
		if role != string(esv1.RemoteClusterClientRole) {
			updatedRoles = append(updatedRoles, role)
		}
	}
	return updatedRoles
}

// validateQuantities ensures that a quantity range is valid.
func validateQuantities(
	errs field.ErrorList,
	autoscalingSpecPath SpecPathBuilder,
	quantityRange *v1alpha1.QuantityRange,
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
				autoscalingSpecPath(index, resource, "min"),
				fmt.Sprintf("min quantity must be greater than %s", minQuantity.String())),
		)
	}

	// A quantity must always be greater than 0.
	if !(quantityRange.Min.Value() > 0) {
		quantityErrs = append(
			quantityErrs,
			field.Required(
				autoscalingSpecPath(index, resource, "min"),
				"min quantity must be greater than 0"),
		)
	}

	if quantityRange.Min.Cmp(quantityRange.Max) > 0 {
		quantityErrs = append(
			quantityErrs,
			field.Invalid(
				autoscalingSpecPath(index, resource, "max"), quantityRange.Max.String(),
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
// a copy of the volume claim template in that case.
func HasAtMostOnePersistentVolumeClaim(nodeSet esv1.NodeSet) (bool, *corev1.PersistentVolumeClaim) {
	switch len(nodeSet.VolumeClaimTemplates) {
	case 0:
		return true, nil
	case 1:
		return true, nodeSet.VolumeClaimTemplates[0].DeepCopy()
	}
	return false, nil
}
