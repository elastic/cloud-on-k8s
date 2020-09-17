// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	shouldRecreate, err := needsRecreate(expectedSset, actualSset)
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
func needsRecreate(expectedSset appsv1.StatefulSet, actualSset appsv1.StatefulSet) (bool, error) {
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
		if isExpansion {
			recreate = true
		}
	}

	return recreate, nil
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
		return false, fmt.Errorf("decreasing storage size is not supported, "+
			"but an attempt was made to resize from %s to %s", actualSize.String(), expectedSize.String())
	default: // increase
		return true, nil
	}
}
