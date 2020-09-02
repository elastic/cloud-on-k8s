// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"fmt"
	"strings"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GarbageCollectPVCs ensures PersistentVolumeClaims created for the given es resource are deleted
// when no longer used, since this is not done automatically by the StatefulSet controller.
// Related issue in the k8s repo: https://github.com/kubernetes/kubernetes/issues/55045
// PVCs that are not supposed to exist given the actual and expected StatefulSets are removed.
// This covers:
// * leftover PVCs created for StatefulSets that do not exist anymore
// * leftover PVCs created for StatefulSets replicas that don't exist anymore (eg. downscale from 5 to 3 nodes)
func GarbageCollectPVCs(
	k8sClient k8s.Client,
	es esv1.Elasticsearch,
	actualStatefulSets sset.StatefulSetList,
	expectedStatefulSets sset.StatefulSetList,
) error {
	// PVCs are using the same labels as their corresponding StatefulSet, so we can filter on ES cluster name.
	var pvcs corev1.PersistentVolumeClaimList
	ns := client.InNamespace(es.Namespace)
	matchLabels := label.NewLabelSelectorForElasticsearch(es)
	if err := k8sClient.List(&pvcs, ns, matchLabels); err != nil {
		return err
	}
	for _, pvc := range pvcsToRemove(pvcs.Items, actualStatefulSets, expectedStatefulSets) {
		log.Info("Deleting PVC", "namespace", pvc.Namespace, "pvc_name", pvc.Name)
		if err := k8sClient.Delete(&pvc); err != nil {
			return err
		}
	}
	return nil
}

// pvcsToRemove filters the given pvcs to ones that can be safely removed based on Pods
// of actual and expected StatefulSets.
func pvcsToRemove(
	pvcs []corev1.PersistentVolumeClaim,
	actualStatefulSets sset.StatefulSetList,
	expectedStatefulSets sset.StatefulSetList,
) []corev1.PersistentVolumeClaim {
	// Build the list of PVCs from both actual & expected StatefulSets (may contain duplicate entries).
	// The list may contain PVCs for Pods that do not exist (eg. not created yet), but does not
	// consider Pods in the process of being deleted (but not deleted yet), since already covered
	// by checking expectations earlier in the process.
	// Then, just return existing PVCs that are not part of that list.
	toKeep := stringsutil.SliceToMap(append(actualStatefulSets.PVCNames(), expectedStatefulSets.PVCNames()...))
	var toRemove []corev1.PersistentVolumeClaim // nolint
	for _, pvc := range pvcs {
		if _, exists := toKeep[pvc.Name]; exists {
			continue
		}
		toRemove = append(toRemove, pvc)
	}
	return toRemove
}

func pvcsToResize(statefulSet appsv1.StatefulSet) (map[string]resource.Quantity, error) {
	names := make(map[string]resource.Quantity, 0)
	for key, value := range statefulSet.Annotations {
		if !strings.HasPrefix(key, nodespec.ResizedVolumeAnnotationName) {
			continue
		}
		claimName := strings.TrimPrefix(key, nodespec.ResizedVolumeAnnotationName)
		for _, podName := range sset.PodNames(statefulSet) {
			pvcName := fmt.Sprintf("%s-%s", claimName, podName)
			quantity, err := resource.ParseQuantity(value)
			if err != nil {
				return nil, err
			}
			names[pvcName] = quantity
		}
	}
	return names, nil
}

func resizePVCs(c k8s.Client, statefulSet appsv1.StatefulSet) error {
	toResize, err := pvcsToResize(statefulSet)
	if err != nil {
		return err
	}
	for pvcName, newStorage := range toResize {
		var pvc corev1.PersistentVolumeClaim
		err := c.Get(types.NamespacedName{Namespace: statefulSet.Namespace, Name: pvcName}, &pvc)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if !pvc.DeletionTimestamp.IsZero() {
			// pvc is scheduled for deletion, there's no point in resizing it
			continue
		}
		existingStorage := pvc.Spec.Resources.Requests.Storage()
		if existingStorage != nil && !existingStorage.Equal(newStorage) {
			log.Info("Resizing PVC storage requests",
				"namespace", statefulSet.Namespace, "pvc_name", pvcName,
				"old_value", existingStorage.String(), "new_value", newStorage.String())
			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = newStorage
			if err := c.Update(&pvc); err != nil {
				return err
			}
		}
	}
	return nil
}
