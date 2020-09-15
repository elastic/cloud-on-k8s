// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"errors"
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// handleVolumeExpansion works around the immutability of VolumeClaimTemplates in StatefulSets by:
// 1. updating storage requests in PVCs whose storage class supports volume expansion
// 2. deleting the StatefulSet, to be recreated with the new storage spec
// It returns a boolean indicating whether the StatefulSet was deleted.
// Note that some storage drivers also require Pods to be deleted/recreated for the filesystem to be resized
// (as opposed to a hot resize while the Pod is running). This is left to the responsibility of the user.
// This should be handled differently once supported by the StatefulSet controller: https://github.com/kubernetes/kubernetes/issues/68737.
func handleVolumeExpansion(k8sClient k8s.Client, expectedSset appsv1.StatefulSet, actualSset appsv1.StatefulSet) (bool, error) {
	err := resizePVCs(k8sClient, expectedSset, actualSset)
	if err != nil {
		return false, err
	}
	return deleteSsetForClaimResize(k8sClient, expectedSset, actualSset)
}

// resizePVCs updates the spec of all existing PVCs whose storage requests can be expanded,
// according to their storage class and what's specified in the expected claim.
// It returns an error if the requested storage size is incompatible with the PVC.
func resizePVCs(k8sClient k8s.Client, expectedSset appsv1.StatefulSet, actualSset appsv1.StatefulSet) error {
	// match each existing PVC with an expected claim, and decide whether the PVC should be resized
	actualPVCs, err := sset.RetrieveActualPVCs(k8sClient, actualSset)
	if err != nil {
		return err
	}
	for claimName, pvcs := range actualPVCs {
		expectedClaim := sset.GetClaim(expectedSset.Spec.VolumeClaimTemplates, claimName)
		if expectedClaim == nil {
			continue
		}
		for _, pvc := range pvcs {
			pvcSize := pvc.Spec.Resources.Requests.Storage()
			claimSize := expectedClaim.Spec.Resources.Requests.Storage()
			// is it a storage increase?
			isExpansion, err := isStorageExpansion(claimSize, pvcSize)
			if err != nil {
				return err
			}
			if !isExpansion {
				continue
			}
			// does the storage class allow volume expansion?
			if err := ensureClaimSupportsExpansion(k8sClient, *expectedClaim); err != nil {
				return err
			}

			log.Info("Resizing PVC storage requests. Depending on the volume provisioner, "+
				"Pods may need to be manually deleted for the filesystem to be resized.",
				"namespace", pvc.Namespace, "pvc_name", pvc.Name,
				"old_value", pvcSize.String(), "new_value", claimSize.String())
			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = *claimSize
			if err := k8sClient.Update(&pvc); err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteSsetForClaimResize compares expected vs. actual StatefulSets, and deletes the actual one
// if a volume expansion can be performed. Pods remain orphan until the StatefulSet is created again.
func deleteSsetForClaimResize(k8sClient k8s.Client, expectedSset appsv1.StatefulSet, actualSset appsv1.StatefulSet) (bool, error) {
	shouldRecreate, err := needsRecreate(k8sClient, expectedSset, actualSset)
	if err != nil {
		return false, err
	}
	if !shouldRecreate {
		return false, nil
	}

	log.Info("Deleting StatefulSet to account for resized PVCs, it will be recreated automatically",
		"namespace", actualSset.Namespace, "statefulset_name", actualSset.Name)

	opts := client.DeleteOptions{}
	// ensure Pods are not also deleted
	orphanPolicy := metav1.DeletePropagationOrphan
	opts.PropagationPolicy = &orphanPolicy
	// ensure we are not deleting based on out-of-date sset spec
	opts.Preconditions = &metav1.Preconditions{
		UID:             &actualSset.UID,
		ResourceVersion: &actualSset.ResourceVersion,
	}
	return true, k8sClient.Delete(&actualSset, &opts)
}

// needsRecreate returns true if the StatefulSet needs to be re-created to account for volume expansion.
// An error is returned if volume expansion is required but claims are incompatible.
func needsRecreate(k8sClient k8s.Client, expectedSset appsv1.StatefulSet, actualSset appsv1.StatefulSet) (bool, error) {
	recreate := false
	// match each expected claim with an actual existing one: we want to return true
	// if at least one claim has increased storage reqs
	// however we want to error-out if any claim has an incompatible storage req
	for _, expectedClaim := range expectedSset.Spec.VolumeClaimTemplates {
		actualClaim := sset.GetClaim(actualSset.Spec.VolumeClaimTemplates, expectedClaim.Name)
		if actualClaim == nil {
			continue
		}
		isExpansion, err := isStorageExpansion(expectedClaim.Spec.Resources.Requests.Storage(), actualClaim.Spec.Resources.Requests.Storage())
		if err != nil {
			return false, err
		}
		if !isExpansion {
			continue
		}
		if err := ensureClaimSupportsExpansion(k8sClient, *actualClaim); err != nil {
			return false, err
		}
		recreate = true
	}

	return recreate, nil
}

// ensureClaimSupportsExpansion inspects whether the storage class referenced by the claim
// allows volume expansion.
func ensureClaimSupportsExpansion(k8sClient k8s.Client, claim corev1.PersistentVolumeClaim) error {
	sc, err := getStorageClass(k8sClient, claim)
	if err != nil {
		return err
	}
	if !allowsVolumeExpansion(sc) {
		return fmt.Errorf("claim %s does not support volume expansion", claim.Name)
	}
	return nil
}

// isStorageExpansion returns true if actual is higher than expected.
// Decreasing storage size is unsupported: an error is returned if expected < actual.
func isStorageExpansion(expectedSize *resource.Quantity, actualSize *resource.Quantity) (bool, error) {
	if expectedSize == nil || actualSize == nil {
		// not much to compare if storage size is unspecified
		return false, nil
	}
	switch expectedSize.Cmp(*actualSize) {
	case 0: // same size
		return false, nil
	case -1: // decrease
		return false, fmt.Errorf("storage size cannot be decreased from %s to %s", actualSize.String(), expectedSize.String())
	default: // increase
		return true, nil
	}
}

// getStorageClass returns the storage class specified by the given claim,
// or the default storage class if the claim does not specify any.
func getStorageClass(k8sClient k8s.Client, claim corev1.PersistentVolumeClaim) (storagev1.StorageClass, error) {
	if claim.Spec.StorageClassName == nil || *claim.Spec.StorageClassName == "" {
		return getDefaultStorageClass(k8sClient)
	}
	var sc storagev1.StorageClass
	if err := k8sClient.Get(types.NamespacedName{Name: *claim.Spec.StorageClassName}, &sc); err != nil {
		return storagev1.StorageClass{}, fmt.Errorf("cannot retrieve storage class: %w", err)
	}
	return sc, nil
}

// getDefaultStorageClass returns the default storage class in the current k8s cluster,
// or an error if there is none.
func getDefaultStorageClass(k8sClient k8s.Client) (storagev1.StorageClass, error) {
	var scs storagev1.StorageClassList
	if err := k8sClient.List(&scs); err != nil {
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
