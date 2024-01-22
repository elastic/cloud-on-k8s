// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package statefulset

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// PodName returns the name of the pod with the given ordinal for this StatefulSet.
func PodName(ssetName string, ordinal int32) string {
	return fmt.Sprintf("%s-%d", ssetName, ordinal)
}

// PodNames returns the names of the pods for this StatefulSet, according to the number of replicas.
func PodNames(sset appsv1.StatefulSet) []string {
	names := make([]string, 0, GetReplicas(sset))
	for i := int32(0); i < GetReplicas(sset); i++ {
		names = append(names, PodName(sset.Name, i))
	}
	return names
}

// GetReplicas returns the replicas configured for this StatefulSet, or 0 if nil.
func GetReplicas(statefulSet appsv1.StatefulSet) int32 {
	if statefulSet.Spec.Replicas != nil {
		return *statefulSet.Spec.Replicas
	}
	return 0
}

// GetClaim returns a pointer to the claim with the given name, or nil if not found.
func GetClaim(claims []corev1.PersistentVolumeClaim, claimName string) *corev1.PersistentVolumeClaim {
	for i, claim := range claims {
		if claim.Name == claimName {
			return &claims[i]
		}
	}
	return nil
}

// RetrieveActualPVCs returns all existing PVCs for that StatefulSet, per claim name.
func RetrieveActualPVCs(k8sClient k8s.Client, statefulSet appsv1.StatefulSet) (map[string][]corev1.PersistentVolumeClaim, error) {
	pvcs := make(map[string][]corev1.PersistentVolumeClaim)
	for _, podName := range PodNames(statefulSet) {
		for _, claim := range statefulSet.Spec.VolumeClaimTemplates {
			if claim.Name == "" {
				continue
			}
			pvcName := fmt.Sprintf("%s-%s", claim.Name, podName)
			var pvc corev1.PersistentVolumeClaim
			if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: statefulSet.Namespace, Name: pvcName}, &pvc); err != nil {
				if apierrors.IsNotFound(err) {
					continue // PVC does not exist (yet)
				}
				return nil, err
			}
			if _, exists := pvcs[claim.Name]; !exists {
				pvcs[claim.Name] = make([]corev1.PersistentVolumeClaim, 0)
			}
			pvcs[claim.Name] = append(pvcs[claim.Name], pvc)
		}
	}
	return pvcs, nil
}
