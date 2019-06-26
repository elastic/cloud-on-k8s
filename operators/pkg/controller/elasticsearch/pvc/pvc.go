// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pvc

import (
	"errors"
	"reflect"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log                         = logf.Log.WithName("pvc")
	ErrNotNodeNameLabelNotFound = errors.New("node name not found as a label on the PVC")
	// PodLabelsInPVCs is the list of labels PVCs inherit from pods they are associated with
	PodLabelsInPVCs = []string{
		label.ClusterNameLabelName,
		common.TypeLabelName,
		string(label.NodeTypesMasterLabelName),
		string(label.NodeTypesIngestLabelName),
		string(label.NodeTypesDataLabelName),
		string(label.NodeTypesMLLabelName),
		label.VersionLabelName,
	}
	// requiredLabelMatch is the list of labels for which PVC values must match their reference values to trigger PVC reuse
	requiredLabelMatch = []string{
		label.ClusterNameLabelName,
		common.TypeLabelName,
		string(label.NodeTypesMasterLabelName),
		string(label.NodeTypesDataLabelName),
		string(label.VolumeNameLabelName),
	}
)

type OrphanedPersistentVolumeClaims struct {
	orphanedPersistentVolumeClaims []corev1.PersistentVolumeClaim
}

// ListVolumeClaims lists the persistent volume claims for the given Elasticsearch cluster.
func ListVolumeClaims(c k8s.Client, es v1alpha1.Elasticsearch) ([]corev1.PersistentVolumeClaim, error) {
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
	return persistentVolumeClaims.Items, nil

}

// FindOrphanedVolumeClaims returns PVC which are not used in any Pod within a given namespace
func FindOrphanedVolumeClaims(
	c k8s.Client,
	es v1alpha1.Elasticsearch,
) (*OrphanedPersistentVolumeClaims, error) {

	persistentVolumeClaims, err := ListVolumeClaims(c, es)
	if err != nil {
		return nil, err
	}

	// Maintain a map of the retrieved PVCs
	pvcByName := map[string]corev1.PersistentVolumeClaim{}
	for _, p := range persistentVolumeClaims {
		if p.DeletionTimestamp != nil {
			continue // PVC is being deleted, ignore it
		}
		pvcByName[p.Name] = p
	}

	// List running pods
	labelSelector := label.NewLabelSelectorForElasticsearch(es)
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
	claim *corev1.PersistentVolumeClaim,
) *corev1.PersistentVolumeClaim {
	for i := 0; i < len(o.orphanedPersistentVolumeClaims); i++ {
		candidate := o.orphanedPersistentVolumeClaims[i]
		if compareLabels(claim.Labels, candidate.Labels) &&
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
	if claim.Spec.StorageClassName == nil {
		// volumeClaimTemplate has no storageClass set: it should use the k8s cluster default
		// since we don't know that default, we fallback to reusing any available volume
		// from the same cluster (whatever the storage class actually is)
		return true
	}
	return reflect.DeepEqual(claim.Spec.StorageClassName, candidate.Spec.StorageClassName)
}

// compareLabels returns true if pvc labels match expectd pvc labels.
// It does not perform a strict comparison, but just compares the expected labels.
// Both expected pvc and existing pvc are allowed to have more labels than the expected ones.
// It also explicitly compares the Elasticsearch version, to make sure we don't
// run a old ES version with data from a newer ES version.
func compareLabels(expectedLabels map[string]string, actualLabels map[string]string) bool {
	// compare subset of labels that must match
	for _, k := range requiredLabelMatch {
		valueInActual, existsInActual := actualLabels[k]
		valueInExpected, existsInExpected := expectedLabels[k]
		if !existsInExpected || !existsInActual || valueInExpected != valueInActual {
			return false
		}
	}
	// only allow pvc to be used for a same or higher version of Elasticsearch
	expectedVersion, err := version.Parse(expectedLabels[label.VersionLabelName])
	if err != nil {
		log.Error(err, "Invalid version in labels", "key", label.VersionLabelName, "value", label.VersionLabelName)
		return false
	}
	actualVersion, err := version.Parse(actualLabels[label.VersionLabelName])
	if err != nil {
		log.Error(err, "Invalid version in labels", "key", label.VersionLabelName, "value", label.VersionLabelName)
		return false
	}
	if !expectedVersion.IsSameOrAfter(*actualVersion) {
		// we are trying to run Elasticsearch with data from a newer version
		return false
	}
	return true
}

func GetPodNameFromLabels(pvc *corev1.PersistentVolumeClaim) (string, error) {
	if name, ok := pvc.Labels[label.PodNameLabelName]; ok {
		return name, nil
	}
	return "", ErrNotNodeNameLabelNotFound
}
