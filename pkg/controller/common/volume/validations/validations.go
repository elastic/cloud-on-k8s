// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	PvcImmutableErrMsg = "volume claim templates can only have their storage requests increased, if the storage class allows volume expansion. Any other change outside of labels modification is forbidden"

	// eckLabelDomainSuffix matches the apex domain (k8s.elastic.co) and any
	// subdomain (e.g. elasticsearch.k8s.elastic.co, common.k8s.elastic.co,
	// association.k8s.elastic.co) that ECK uses for operator-managed labels.
	eckLabelDomainSuffix = "k8s.elastic.co"
)

// IsReservedLabelKey reports whether key is owned by ECK and must not be set
// or modified by users via VolumeClaimTemplate labels. ECK-reserved keys live
// under any *.k8s.elastic.co/ domain (e.g. elasticsearch.k8s.elastic.co/cluster-name,
// common.k8s.elastic.co/type, association.k8s.elastic.co/...). Overwriting them
// on a PVC would break PVC GC and owner-ref reconciliation, which select on
// ClusterNameLabelName / StatefulSetNameLabelName.
func IsReservedLabelKey(key string) bool {
	domain, _, ok := strings.Cut(key, "/")
	if !ok {
		return false
	}
	return domain == eckLabelDomainSuffix || strings.HasSuffix(domain, "."+eckLabelDomainSuffix)
}

// StripReservedLabelKeys returns a deep copy of claims with all ECK-reserved label keys
// removed from each claim's metadata.labels. This is the reconciler-side complement to
// the create-time webhook check (validPVCReservedLabelsOnCreate / checkPVCReservedLabelsOnCreate):
// it provides defense-in-depth so that operators running with webhooks disabled, or CRs
// that otherwise bypass admission, still cannot leak reserved label keys onto freshly
// provisioned PVCs (the StatefulSet controller copies VCT metadata to PVCs at creation time).
//
// Claims are cloned only when at least one reserved key is present; the returned slice is
// safe to mutate. If no reserved keys are present, the input slice is returned unchanged.
func StripReservedLabelKeys(claims []corev1.PersistentVolumeClaim) []corev1.PersistentVolumeClaim {
	if !anyClaimCarriesReservedKey(claims) {
		return claims
	}
	out := make([]corev1.PersistentVolumeClaim, 0, len(claims))
	for _, claim := range claims {
		c := *claim.DeepCopy()
		for k := range c.ObjectMeta.Labels {
			if IsReservedLabelKey(k) {
				delete(c.ObjectMeta.Labels, k)
			}
		}
		// Drop empty maps so the resulting object is byte-equivalent to one that never
		// carried labels in the first place (avoids spurious StatefulSet diff on update).
		if len(c.ObjectMeta.Labels) == 0 {
			c.ObjectMeta.Labels = nil
		}
		out = append(out, c)
	}
	return out
}

func anyClaimCarriesReservedKey(claims []corev1.PersistentVolumeClaim) bool {
	for _, claim := range claims {
		for k := range claim.ObjectMeta.Labels {
			if IsReservedLabelKey(k) {
				return true
			}
		}
	}
	return false
}

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
			// updated declares a claim that does not exist in initial: adding new claims is forbidden.
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
