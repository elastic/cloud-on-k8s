// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"fmt"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/autoscaling"
	volumevalidations "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume/validations"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const reservedPVCLabelKeyErrMsgFmt = "label key %q is reserved by ECK and cannot be set on volumeClaimTemplates"

func validPVCNaming(proposed esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	for i, ns := range proposed.Spec.NodeSets {
		// do we have a claim at all and if so is it named correctly? OK
		if len(ns.VolumeClaimTemplates) == 0 || hasDefaultClaim(ns.VolumeClaimTemplates, proposed.IsStateless()) {
			continue
		}
		// we have claims but they are using custom names, are all of them mounted as a volume?
		for _, m := range unmountedClaims(ns) {
			msg := pvcNotMountedStatefulErrMsg
			if proposed.IsStateless() {
				msg = pvcNotMountedStatelessErrMsg
			}
			errs = append(errs, field.Invalid(
				field.NewPath("spec").Child("nodeSets").Index(i).Child("volumeClaimTemplates"),
				m.Name,
				msg,
			))
		}
	}
	return errs
}

func unmountedClaims(ns esv1.NodeSet) []corev1.PersistentVolumeClaim {
	templates := ns.VolumeClaimTemplates
	for _, c := range ns.PodTemplate.Spec.Containers {
		for _, vm := range c.VolumeMounts {
			for i := len(templates) - 1; i >= 0; i-- {
				if templates[i].Name == vm.Name {
					templates = append(templates[:i], templates[i+1:]...)
				}
			}
		}
	}
	return templates
}

func hasDefaultClaim(templates []corev1.PersistentVolumeClaim, isStateless bool) bool {
	for _, t := range templates {
		if !isStateless && t.Name == volume.ElasticsearchDataVolumeName {
			return true
		}
		if isStateless && t.Name == volume.ElasticsearchCacheVolumeName {
			return true
		}
	}
	return false
}

// validPVCModification ensures only the adjustable parts of volume claim templates are changed.
// The currently adjustable fields are:
//   - spec.resources.requests.storage: increases are allowed when the storage class supports
//     volume expansion; decreases are rejected unless the matching StatefulSet still has the
//     "old" storage size (revert scenario).
//   - metadata.labels: free-form user labels can be added, modified, or removed (additive-only
//     propagation to existing PVCs is performed by HandleVolumeExpansion). Reserved ECK label
//     keys (see validPVCReservedLabels) are rejected by a separate validation.
//
// Any other change to a VolumeClaimTemplate field is forbidden.
func validPVCModification(ctx context.Context, current esv1.Elasticsearch, proposed esv1.Elasticsearch, k8sClient k8s.Client, validateStorageClass bool) field.ErrorList {
	log := ulog.FromContext(ctx)
	var errs field.ErrorList

	autoscalingResource, err := autoscaling.GetAssociatedAutoscalingResource(ctx, k8sClient, proposed)
	if err != nil {
		log.Error(
			err,
			"Error while trying to check if this cluster is managed by the autoscaling controller, skip volume validation",
			"namespace", proposed.Namespace,
			"es_name", proposed.Name,
		)
		return errs
	}

	if autoscalingResource != nil {
		// If a resource manifest is applied without a volume claim or with an old volume claim template, the NodeSet specification
		// will not be processed immediately by the Elasticsearch controller. When autoscaling is enabled it is fine to accept the
		// manifest, and wait for the autoscaling controller to adjust the volume claim template size.
		log.V(1).Info(
			"Autoscaling is enabled in proposed, ignoring PVC modification validation",
			"namespace", proposed.Namespace,
			"es_name", proposed.Name,
		)
		return errs
	}
	for i, proposedNodeSet := range proposed.Spec.NodeSets {
		currentNodeSet := getNodeSet(proposedNodeSet.Name, current)
		if currentNodeSet != nil {
			// Check that no modification was made to the claims, except on storage requests and labels.
			if !apiequality.Semantic.DeepEqual(
				claimsWithoutAdjustableFields(currentNodeSet.VolumeClaimTemplates),
				claimsWithoutAdjustableFields(proposedNodeSet.VolumeClaimTemplates),
			) {
				errs = append(errs, field.Invalid(
					field.NewPath("spec").Child("nodeSets").Index(i).Child("volumeClaimTemplates"),
					proposedNodeSet.VolumeClaimTemplates,
					volumevalidations.PvcImmutableErrMsg,
				))
				continue
			}
		}

		// Allow storage increase to go through if the storage class supports volume expansion.
		// Reject storage decrease, unless the matching StatefulSet still has the "old" storage:
		// we want to cover the case where the user upgrades storage from 1GB to 2GB, then realizes the reconciliation
		// errors out for some reasons, then reverts the storage size to a correct 1GB. In that case the StatefulSet
		// claim is still configured with 1GB even though the current Elasticsearch specifies 2GB.
		// Hence here we compare proposed claims with **current StatefulSet** claims.
		matchingSsetName := esv1.StatefulSet(proposed.Name, proposedNodeSet.Name)
		var matchingSset appsv1.StatefulSet
		err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: proposed.Namespace, Name: matchingSsetName}, &matchingSset)
		if err != nil && apierrors.IsNotFound(err) {
			// matching StatefulSet does not exist, this is likely the initial creation
			continue
		} else if err != nil {
			// k8s client error - unlikely to happen since we used a cached client, but if it does happen
			// we don't want to return an admission error here.
			// In doubt, validate the request. Validation is performed again during the reconciliation.
			log.Error(err, "error while validating pvc modification, skipping validation")
			continue
		}

		if currentNodeSet == nil {
			// this means that there is a StatefulSet with matchingSsetName but no nodeSet with this name in the current Elasticsearch.
			// This can happen by quick renames of nodeSets in the Elasticsearch resource.
			errs = append(errs, field.Invalid(
				field.NewPath("spec").Child("nodeSets").Index(i).Child("name"),
				proposedNodeSet.Name,
				"cannot reuse nodeSet name while a StatefulSet with that name still exists from a previous configuration",
			))
			continue
		}

		if err := volumevalidations.ValidateClaimsStorageUpdate(ctx, k8sClient, matchingSset.Spec.VolumeClaimTemplates, proposedNodeSet.VolumeClaimTemplates, validateStorageClass); err != nil {
			errs = append(errs, field.Invalid(
				field.NewPath("spec").Child("nodeSets").Index(i).Child("volumeClaimTemplates"),
				proposedNodeSet.VolumeClaimTemplates,
				err.Error(),
			))
		}
	}
	return errs
}

func getNodeSet(name string, es esv1.Elasticsearch) *esv1.NodeSet {
	for i := range es.Spec.NodeSets {
		if es.Spec.NodeSets[i].Name == name {
			return &es.Spec.NodeSets[i]
		}
	}
	return nil
}

// validPVCReservedLabels rejects volumeClaimTemplate label entries that introduce or
// modify ECK-reserved label keys (anything under the *.k8s.elastic.co/ domain). Such
// labels would propagate to the resulting PVCs and could break PVC GC and owner-ref
// reconciliation, which select on elasticsearch.k8s.elastic.co/cluster-name and
// elasticsearch.k8s.elastic.co/statefulset-name.
//
// Runs on update only and grandfathers (key, value) pairs already present on the matching
// current claim so existing clusters that carry such labels are not bricked at upgrade
// time. New clusters bypass this admission check; the reconciler's defensive guard in
// syncPVCLabels still strips any reserved key before applying labels to PVCs.
func validPVCReservedLabels(current, proposed esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	for i, proposedNodeSet := range proposed.Spec.NodeSets {
		currentNodeSet := getNodeSet(proposedNodeSet.Name, current)
		for j, proposedClaim := range proposedNodeSet.VolumeClaimTemplates {
			currentClaim := matchingClaim(currentNodeSet, proposedClaim.Name)
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
					field.NewPath("spec").Child("nodeSets").Index(i).
						Child("volumeClaimTemplates").Index(j).
						Child("metadata", "labels").Key(key),
					key,
					fmt.Sprintf(reservedPVCLabelKeyErrMsgFmt, key),
				))
			}
		}
	}
	return errs
}

// matchingClaim returns the claim with the given name in the nodeSet, or nil if not found
// or the nodeSet itself is nil.
func matchingClaim(nodeSet *esv1.NodeSet, name string) *corev1.PersistentVolumeClaim {
	if nodeSet == nil {
		return nil
	}
	for i := range nodeSet.VolumeClaimTemplates {
		if nodeSet.VolumeClaimTemplates[i].Name == name {
			return &nodeSet.VolumeClaimTemplates[i]
		}
	}
	return nil
}

// claimsWithoutAdjustableFields returns a copy of the given claims, with all known
// adjustable fields cleared so unrelated changes can be compared via deep equality.
func claimsWithoutAdjustableFields(claims []corev1.PersistentVolumeClaim) []corev1.PersistentVolumeClaim {
	result := make([]corev1.PersistentVolumeClaim, 0, len(claims))
	for _, claim := range claims {
		patchedClaim := *claim.DeepCopy()
		// Storage quantity is allowed to be adjusted. Defensively initialize Requests
		// to avoid panicking on malformed claims with a nil Requests map.
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
