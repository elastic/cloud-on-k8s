// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"context"
	"errors"
	"fmt"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// validPVCModification ensures the only part of volume claim templates that can be changed is storage requests.
// Storage increase is allowed as long as the storage class supports volume expansion.
// Storage decrease is not supported if the corresponding StatefulSet has been resized already.
func validPVCModification(current esv1.Elasticsearch, proposed esv1.Elasticsearch, k8sClient k8s.Client, validateStorageClass bool) field.ErrorList {
	var errs field.ErrorList
	if proposed.IsAutoscalingDefined() {
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
				pvcImmutableErrMsg,
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

		if err := ValidateClaimsStorageUpdate(k8sClient, matchingSset.Spec.VolumeClaimTemplates, proposedNodeSet.VolumeClaimTemplates, validateStorageClass); err != nil {
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

// ValidateClaimsStorageUpdate compares updated vs. initial claim, and returns an error if:
// - a storage decrease is attempted
// - a storage increase is attempted but the storage class does not support volume expansion
// - a new claim was added in updated ones
func ValidateClaimsStorageUpdate(
	k8sClient k8s.Client,
	initial []corev1.PersistentVolumeClaim,
	updated []corev1.PersistentVolumeClaim,
	validateStorageClass bool,
) error {
	for _, updatedClaim := range updated {
		initialClaim := claimMatchingName(initial, updatedClaim.Name)
		if initialClaim == nil {
			// existing claim does not exist in updated
			return errors.New(pvcImmutableErrMsg)
		}

		cmp := k8s.CompareStorageRequests(initialClaim.Spec.Resources, updatedClaim.Spec.Resources)
		switch {
		case cmp.Increase:
			// storage increase requested: ensure the storage class allows volume expansion
			if err := EnsureClaimSupportsExpansion(k8sClient, updatedClaim, validateStorageClass); err != nil {
				return err
			}
		case cmp.Decrease:
			// storage decrease is not supported
			return fmt.Errorf("decreasing storage size is not supported: an attempt was made to decrease storage size for claim %s", updatedClaim.Name)
		}
	}
	return nil
}

func claimMatchingName(claims []corev1.PersistentVolumeClaim, name string) *corev1.PersistentVolumeClaim {
	for i, claim := range claims {
		if claim.Name == name {
			return &claims[i]
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

// EnsureClaimSupportsExpansion inspects whether the storage class referenced by the claim
// allows volume expansion, and returns an error if it doesn't.
func EnsureClaimSupportsExpansion(k8sClient k8s.Client, claim corev1.PersistentVolumeClaim, validateStorageClass bool) error {
	if !validateStorageClass {
		log.V(1).Info("Skipping storage class validation")
		return nil
	}
	sc, err := getStorageClass(k8sClient, claim)
	if err != nil {
		return err
	}
	if !allowsVolumeExpansion(sc) {
		return fmt.Errorf("claim %s (storage class %s) does not support volume expansion", claim.Name, sc.Name)
	}
	return nil
}

// getStorageClass returns the storage class specified by the given claim,
// or the default storage class if the claim does not specify any.
func getStorageClass(k8sClient k8s.Client, claim corev1.PersistentVolumeClaim) (storagev1.StorageClass, error) {
	if claim.Spec.StorageClassName == nil || *claim.Spec.StorageClassName == "" {
		return getDefaultStorageClass(k8sClient)
	}
	var sc storagev1.StorageClass
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: *claim.Spec.StorageClassName}, &sc); err != nil {
		return storagev1.StorageClass{}, fmt.Errorf("cannot retrieve storage class: %w", err)
	}
	return sc, nil
}

// getDefaultStorageClass returns the default storage class in the current k8s cluster,
// or an error if there is none.
func getDefaultStorageClass(k8sClient k8s.Client) (storagev1.StorageClass, error) {
	var scs storagev1.StorageClassList
	if err := k8sClient.List(context.Background(), &scs); err != nil {
		return storagev1.StorageClass{}, err
	}
	for _, sc := range scs.Items {
		if isDefaultStorageClass(sc) {
			return sc, nil
		}
	}
	return storagev1.StorageClass{}, errors.New("no default storage class found")
}

// isDefaultStorageClass inspects the given storage class and returns true if it is annotated as the default one.
func isDefaultStorageClass(sc storagev1.StorageClass) bool {
	if len(sc.Annotations) == 0 {
		return false
	}
	if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
		sc.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
		return true
	}
	return false
}

// allowsVolumeExpansion returns true if the given storage class allows volume expansion.
func allowsVolumeExpansion(sc storagev1.StorageClass) bool {
	return sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion
}
