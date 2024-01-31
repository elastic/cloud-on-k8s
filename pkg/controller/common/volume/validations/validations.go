// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validations

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	PvcImmutableErrMsg = "volume claim templates can only have their storage requests increased, if the storage class allows volume expansion. Any other change is forbidden"
)

// ValidateClaimsStorageUpdate compares updated vs. initial claim, and returns an error if:
// - a storage decrease is attempted
// - a storage increase is attempted but the storage class does not support volume expansion
// - a new claim was added in updated ones
func ValidateClaimsStorageUpdate(
	ctx context.Context,
	k8sClient k8s.Client,
	initial []corev1.PersistentVolumeClaim,
	updated []corev1.PersistentVolumeClaim,
	validateStorageClass bool,
) error {
	for _, updatedClaim := range updated {
		initialClaim := claimMatchingName(initial, updatedClaim.Name)
		if initialClaim == nil {
			// existing claim does not exist in updated
			return errors.New(PvcImmutableErrMsg)
		}
		cmp := k8s.CompareStorageRequests(initialClaim.Spec.Resources, updatedClaim.Spec.Resources)
		switch {
		case cmp.Increase:
			// storage increase requested: ensure the storage class allows volume expansion
			if err := EnsureClaimSupportsExpansion(ctx, k8sClient, updatedClaim, validateStorageClass); err != nil {
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

// EnsureClaimSupportsExpansion inspects whether the storage class referenced by the claim
// allows volume expansion, and returns an error if it doesn't.
func EnsureClaimSupportsExpansion(ctx context.Context, k8sClient k8s.Client, claim corev1.PersistentVolumeClaim, validateStorageClass bool) error {
	if !validateStorageClass {
		ulog.FromContext(ctx).V(1).Info("Skipping storage class validation")
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
