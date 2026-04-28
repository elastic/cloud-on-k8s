// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	volumevalidations "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume/validations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	pvcImmutableMsg              = "Volume claim templates can only have their storage requests and labels modified"
	reservedPVCLabelKeyErrMsgFmt = "label key %q is reserved by ECK and cannot be set on volumeClaimTemplates"
)

type validation func(*lsv1alpha1.Logstash) field.ErrorList

type updateValidation func(*lsv1alpha1.Logstash, *lsv1alpha1.Logstash) field.ErrorList

// updateValidations are the validation funcs that only apply to updates
func updateValidations(ctx context.Context, k8sClient k8s.Client, validateStorageClass bool) []updateValidation {
	return []updateValidation{
		checkNoDowngrade,
		func(current *lsv1alpha1.Logstash, proposed *lsv1alpha1.Logstash) field.ErrorList {
			return checkPVCchanges(ctx, current, proposed, k8sClient, validateStorageClass)
		},
		checkPVCReservedLabels,
	}
}

// validations are the validation funcs that apply to creates or updates
func validations() []validation {
	return []validation{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkSingleConfigSource,
		checkESRefsNamed,
		checkAssociations,
		checkSinglePipelineSource,
	}
}

func checkNoUnknownFields(l *lsv1alpha1.Logstash) field.ErrorList {
	return commonv1.NoUnknownFields(l, l.ObjectMeta)
}

func checkNameLength(l *lsv1alpha1.Logstash) field.ErrorList {
	return commonv1.CheckNameLength(l)
}

func checkSupportedVersion(l *lsv1alpha1.Logstash) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(l.Spec.Version, version.SupportedLogstashVersions)
}

func checkNoDowngrade(prev, curr *lsv1alpha1.Logstash) field.ErrorList {
	if commonv1.IsConfiguredToAllowDowngrades(curr) {
		return nil
	}
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}

func checkSingleConfigSource(l *lsv1alpha1.Logstash) field.ErrorList {
	if l.Spec.Config != nil && l.Spec.ConfigRef != nil {
		msg := "Specify at most one of [`config`, `configRef`], not both"
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("config"), msg),
			field.Forbidden(field.NewPath("spec").Child("configRef"), msg),
		}
	}

	return nil
}

func checkAssociations(l *lsv1alpha1.Logstash) field.ErrorList {
	monitoringPath := field.NewPath("spec").Child("monitoring")
	err1 := commonv1.CheckAssociationRefs(monitoringPath.Child("metrics"), l.GetMonitoringMetricsRefs()...)
	err2 := commonv1.CheckAssociationRefs(monitoringPath.Child("logs"), l.GetMonitoringLogsRefs()...)
	err3 := commonv1.CheckElasticsearchSelectorRefs(field.NewPath("spec").Child("elasticsearchRefs"), l.ElasticsearchRefs()...)
	return append(append(err1, err2...), err3...)
}

func checkSinglePipelineSource(a *lsv1alpha1.Logstash) field.ErrorList {
	if a.Spec.Pipelines != nil && a.Spec.PipelinesRef != nil {
		msg := "Specify at most one of [`pipelines`, `pipelinesRef`], not both"
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("pipelines"), msg),
			field.Forbidden(field.NewPath("spec").Child("pipelinesRef"), msg),
		}
	}

	return nil
}

func checkESRefsNamed(l *lsv1alpha1.Logstash) field.ErrorList {
	var errorList field.ErrorList
	for i, esRef := range l.Spec.ElasticsearchRefs {
		if esRef.ClusterName == "" {
			errorList = append(
				errorList,
				field.Required(
					field.NewPath("spec").Child("elasticsearchRefs").Index(i).Child("clusterName"),
					fmt.Sprintf("clusterName is a mandatory field - missing on %v", esRef.NamespacedName())),
			)
		}
	}
	return errorList
}

// checkPVCchanges ensures only the adjustable parts of volume claim templates are changed.
// The currently adjustable fields are:
//   - spec.resources.requests.storage: increases are allowed when the storage class supports
//     volume expansion; decreases are rejected.
//   - metadata.labels: free-form user labels can be added, modified, or removed (additive-only
//     propagation to existing PVCs is performed by HandleVolumeExpansion). Reserved ECK label
//     keys (see checkPVCReservedLabels) are rejected by a separate validation.
//
// Any other change to a VolumeClaimTemplate field is forbidden.
func checkPVCchanges(ctx context.Context, current *lsv1alpha1.Logstash, proposed *lsv1alpha1.Logstash, k8sClient k8s.Client, validateStorageClass bool) field.ErrorList {
	var errs field.ErrorList
	if current == nil || proposed == nil {
		return errs
	}

	// Check that no modification was made to the claims, except on storage requests and labels.
	if !apiequality.Semantic.DeepEqual(
		claimsWithoutAdjustableFields(current.Spec.VolumeClaimTemplates),
		claimsWithoutAdjustableFields(proposed.Spec.VolumeClaimTemplates),
	) {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("volumeClaimTemplates"), proposed.Spec.VolumeClaimTemplates, pvcImmutableMsg))
	}

	if err := volumevalidations.ValidateClaimsStorageUpdate(ctx, k8sClient, current.Spec.VolumeClaimTemplates, proposed.Spec.VolumeClaimTemplates, validateStorageClass); err != nil {
		errs = append(errs, field.Invalid(
			field.NewPath("spec").Child("volumeClaimTemplates"),
			proposed.Spec.VolumeClaimTemplates,
			err.Error(),
		))
	}

	return errs
}

// checkPVCReservedLabels rejects volumeClaimTemplate label entries that introduce or modify
// ECK-reserved label keys (anything under the *.k8s.elastic.co/ domain). Such labels would
// propagate to the resulting PVCs and could break PVC reconciliation paths that rely on
// those keys.
//
// Runs on update only and grandfathers (key, value) pairs already present on the matching
// current claim so existing CRs that carry such labels are not bricked at upgrade time.
// New CRs bypass this admission check; the reconciler's defensive guard in syncPVCLabels
// still strips any reserved key before applying labels to PVCs.
func checkPVCReservedLabels(current, proposed *lsv1alpha1.Logstash) field.ErrorList {
	var errs field.ErrorList
	if current == nil || proposed == nil {
		return errs
	}
	for j, proposedClaim := range proposed.Spec.VolumeClaimTemplates {
		currentClaim := matchingClaim(current.Spec.VolumeClaimTemplates, proposedClaim.Name)
		for key, value := range proposedClaim.ObjectMeta.Labels {
			if !volumevalidations.IsReservedLabelKey(key) {
				continue
			}
			if currentClaim != nil {
				if currentValue, exists := currentClaim.ObjectMeta.Labels[key]; exists && currentValue == value {
					// grandfather pre-existing (key, value) pair
					continue
				}
			}
			errs = append(errs, field.Invalid(
				field.NewPath("spec").Child("volumeClaimTemplates").Index(j).
					Child("metadata", "labels").Key(key),
				key,
				fmt.Sprintf(reservedPVCLabelKeyErrMsgFmt, key),
			))
		}
	}
	return errs
}

// matchingClaim returns the claim with the given name in claims, or nil if not found.
func matchingClaim(claims []corev1.PersistentVolumeClaim, name string) *corev1.PersistentVolumeClaim {
	for i := range claims {
		if claims[i].Name == name {
			return &claims[i]
		}
	}
	return nil
}

// claimsWithoutAdjustableFields returns a copy of the given claims, with all known adjustable
// fields cleared so unrelated changes can be compared via deep equality.
func claimsWithoutAdjustableFields(claims []corev1.PersistentVolumeClaim) []corev1.PersistentVolumeClaim {
	result := make([]corev1.PersistentVolumeClaim, 0, len(claims))
	for _, claim := range claims {
		patchedClaim := *claim.DeepCopy()
		// Storage quantity is allowed to be adjusted.
		if patchedClaim.Spec.Resources.Requests == nil {
			patchedClaim.Spec.Resources.Requests = corev1.ResourceList{}
		}
		patchedClaim.Spec.Resources.Requests[corev1.ResourceStorage] = resource.Quantity{}
		// Labels are allowed to be adjusted.
		patchedClaim.ObjectMeta.Labels = map[string]string{}
		result = append(result, patchedClaim)
	}
	return result
}

func check(ls *lsv1alpha1.Logstash, validations []validation) field.ErrorList {
	var errs field.ErrorList
	for _, val := range validations {
		if err := val(ls); err != nil {
			errs = append(errs, err...)
		}
	}
	return errs
}
