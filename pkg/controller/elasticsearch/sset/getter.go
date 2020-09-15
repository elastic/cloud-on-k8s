// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// GetReplicas returns the replicas configured for this StatefulSet, or 0 if nil.
func GetReplicas(statefulSet appsv1.StatefulSet) int32 {
	if statefulSet.Spec.Replicas != nil {
		return *statefulSet.Spec.Replicas
	}
	return 0
}

// GetESVersion returns the ES version from the StatefulSet labels.
func GetESVersion(statefulSet appsv1.StatefulSet) (*version.Version, error) {
	return label.ExtractVersion(statefulSet.Spec.Template.Labels)
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
			if err := k8sClient.Get(types.NamespacedName{Namespace: statefulSet.Namespace, Name: pvcName}, &pvc); err != nil {
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
