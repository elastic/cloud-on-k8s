// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pvc

import (
	"errors"
	"reflect"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log                         = logf.Log.WithName("pvc")
	standardStorageClassname    = "standard"
	ErrNotNodeNameLabelNotFound = errors.New("node name not found as a label on the PVC")
)

type OrphanedPersistentVolumeClaims struct {
	orphanedPersistentVolumeClaims []corev1.PersistentVolumeClaim
}

// FindOrphanedVolumeClaims returns PVC which are not used in any Pod within a given namespace
func FindOrphanedVolumeClaims(
	c k8s.Client,
	es v1alpha1.ElasticsearchCluster,
) (*OrphanedPersistentVolumeClaims, error) {
	labelSelector := label.NewLabelSelectorForElasticsearch(es)
	// List PVC
	listPVCOptions := client.ListOptions{
		Namespace:     es.Namespace,
		LabelSelector: labelSelector,
	}

	var persistentVolumeClaims corev1.PersistentVolumeClaimList
	if err := c.List(&listPVCOptions, &persistentVolumeClaims); err != nil {
		return nil, err
	}

	// Maintain a map of the retrieved PVCs
	pvcByName := map[string]corev1.PersistentVolumeClaim{}
	for _, p := range persistentVolumeClaims.Items {
		if p.DeletionTimestamp != nil {
			continue // PVC is being deleted, ignore it
		}
		pvcByName[p.Name] = p
	}

	// List running pods
	listPodSOptions := client.ListOptions{
		Namespace:     es.Namespace,
		LabelSelector: labelSelector,
	}

	var pods corev1.PodList
	if err := c.List(&listPodSOptions, &pods); err != nil {
		return nil, err
	}

	// Remove the PVCs that are attached
	for _, p := range pods.Items {
		for _, v := range p.Spec.Volumes {
			if v.PersistentVolumeClaim != nil {
				delete(pvcByName, v.PersistentVolumeClaim.ClaimName)
			}
		}
	}

	// The result is the remaining list of PVC
	orphanedPVCs := make([]corev1.PersistentVolumeClaim, 0, len(pvcByName))
	for _, pvc := range pvcByName {
		orphanedPVCs = append(orphanedPVCs, pvc)
	}

	return &OrphanedPersistentVolumeClaims{orphanedPersistentVolumeClaims: orphanedPVCs}, nil
}

// GetOrphanedVolumeClaim extract and remove a matching existing and orphaned PVC, returns nil if none is found
func (o *OrphanedPersistentVolumeClaims) GetOrphanedVolumeClaim(
	expectedLabels map[string]string,
	claim *corev1.PersistentVolumeClaim,
) *corev1.PersistentVolumeClaim {
	for i := 0; i < len(o.orphanedPersistentVolumeClaims); i++ {
		candidate := o.orphanedPersistentVolumeClaims[i]
		if compareLabels(expectedLabels, candidate.Labels) &&
			compareStorageClass(claim, &candidate) &&
			compareResources(claim, &candidate) {
			o.orphanedPersistentVolumeClaims = append(o.orphanedPersistentVolumeClaims[:i], o.orphanedPersistentVolumeClaims[i+1:]...)
			return &candidate
		}
	}
	return nil
}

// TODO : Should we accept a storage with more space than needed ?
func compareResources(claim, candidate *corev1.PersistentVolumeClaim) bool {
	claimStorage := claim.Spec.Resources.Requests["storage"]
	candidateStorage := candidate.Spec.Resources.Requests["storage"]
	return claimStorage.Cmp(candidateStorage) == 0

}

func compareStorageClass(claim, candidate *corev1.PersistentVolumeClaim) bool {
	if claim.Spec.StorageClassName != nil {
		return reflect.DeepEqual(claim.Spec.StorageClassName, candidate.Spec.StorageClassName)
	}
	// No storage class name in the claim, only match if the claim is a standard storage class
	return standardStorageClassname == *candidate.Spec.StorageClassName
}

// compare two maps but ignore the label.PodNameLabelName key
// TODO : do we really need a strict comparison ?
func compareLabels(labels1 map[string]string, labels2 map[string]string) bool {
	if labels1 == nil || labels2 == nil {
		return false
	}
	if len(labels1) != len(labels2) {
		return false
	}
	for key1, val1 := range labels1 {
		if key1 == label.PodNameLabelName {
			continue
		}
		if val2, ok := labels2[key1]; !ok || val2 != val1 {
			return false
		}
	}
	return true
}

func GetPodNameFromLabels(pvc *corev1.PersistentVolumeClaim) (string, error) {
	if name, ok := pvc.Labels[label.PodNameLabelName]; ok {
		return name, nil
	}
	return "", ErrNotNodeNameLabelNotFound
}
