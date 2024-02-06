// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/autoscaling"
	volumevalidations "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume/validations"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

func validPVCNaming(proposed esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	for i, ns := range proposed.Spec.NodeSets {
		// do we have a claim at all and if so is it named correctly? OK
		if len(ns.VolumeClaimTemplates) == 0 || hasDefaultClaim(ns.VolumeClaimTemplates) {
			continue
		}
		// we have claims but they are using custom names, are all of them mounted as a volume?
		for _, m := range unmountedClaims(ns) {
			errs = append(errs, field.Invalid(
				field.NewPath("spec").Child("nodeSet").Index(i).Child("volumeClaimTemplates"),
				m.Name,
				pvcNotMountedErrMsg,
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

func hasDefaultClaim(templates []corev1.PersistentVolumeClaim) bool {
	for _, t := range templates {
		if t.Name == volume.ElasticsearchDataVolumeName {
			return true
		}
	}
	return false
}

// validPVCModification ensures the only part of volume claim templates that can be changed is storage requests.
// Storage increase is allowed as long as the storage class supports volume expansion.
// Storage decrease is not supported if the corresponding StatefulSet has been resized already.
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
		if currentNodeSet == nil {
			// initial creation
			continue
		}

		// Check that no modification was made to the claims, except on storage requests.
		if !apiequality.Semantic.DeepEqual(
			claimsWithoutStorageReq(currentNodeSet.VolumeClaimTemplates),
			claimsWithoutStorageReq(proposedNodeSet.VolumeClaimTemplates),
		) {
			errs = append(errs, field.Invalid(
				field.NewPath("spec").Child("nodeSet").Index(i).Child("volumeClaimTemplates"),
				proposedNodeSet.VolumeClaimTemplates,
				volumevalidations.PvcImmutableErrMsg,
			))
			continue
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

		if err := volumevalidations.ValidateClaimsStorageUpdate(ctx, k8sClient, matchingSset.Spec.VolumeClaimTemplates, proposedNodeSet.VolumeClaimTemplates, validateStorageClass); err != nil {
			errs = append(errs, field.Invalid(
				field.NewPath("spec").Child("nodeSet").Index(i).Child("volumeClaimTemplates"),
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

// claimsWithoutStorageReq returns a copy of the given claims, with all storage requests set to the empty quantity.
func claimsWithoutStorageReq(claims []corev1.PersistentVolumeClaim) []corev1.PersistentVolumeClaim {
	result := make([]corev1.PersistentVolumeClaim, 0, len(claims))
	for _, claim := range claims {
		patchedClaim := *claim.DeepCopy()
		patchedClaim.Spec.Resources.Requests[corev1.ResourceStorage] = resource.Quantity{}
		result = append(result, patchedClaim)
	}
	return result
}
