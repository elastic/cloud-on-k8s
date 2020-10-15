// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"encoding/json"
	"strings"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/validation"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// RecreateStatefulSetAnnotationPrefix is used to annotate the Elasticsearch resource
	// with StatefulSets to recreate. The StatefulSet name is appended to this name.
	RecreateStatefulSetAnnotationPrefix = "elasticsearch.k8s.elastic.co/recreate-"
)

// handleVolumeExpansion works around the immutability of VolumeClaimTemplates in StatefulSets by:
// 1. updating storage requests in PVCs whose storage class supports volume expansion
// 2. scheduling the StatefulSet for recreation with the new storage spec
// It returns a boolean indicating whether the StatefulSet needs to be recreated.
// Note that some storage drivers also require Pods to be deleted/recreated for the filesystem to be resized
// (as opposed to a hot resize while the Pod is running). This is left to the responsibility of the user.
// This should be handled differently once supported by the StatefulSet controller: https://github.com/kubernetes/kubernetes/issues/68737.
func handleVolumeExpansion(
	k8sClient k8s.Client,
	es esv1.Elasticsearch,
	expectedSset appsv1.StatefulSet,
	actualSset appsv1.StatefulSet,
	validateStorageClass bool,
) (bool, error) {
	// ensure there are no incompatible storage size modification
	if err := validation.ValidateClaimsStorageUpdate(
		k8sClient,
		actualSset.Spec.VolumeClaimTemplates,
		expectedSset.Spec.VolumeClaimTemplates,
		validateStorageClass); err != nil {
		return false, err
	}

	// resize all PVCs that can be resized
	err := resizePVCs(k8sClient, es, expectedSset, actualSset)
	if err != nil {
		return false, err
	}

	// schedule the StatefulSet for recreation if needed
	if needsRecreate(expectedSset, actualSset) {
		return true, annotateForRecreation(k8sClient, es, actualSset, expectedSset.Spec.VolumeClaimTemplates)
	}

	return false, nil
}

// resizePVCs updates the spec of all existing PVCs whose storage requests can be expanded,
// according to their storage class and what's specified in the expected claim.
// It returns an error if the requested storage size is incompatible with the PVC.
func resizePVCs(
	k8sClient k8s.Client,
	es esv1.Elasticsearch,
	expectedSset appsv1.StatefulSet,
	actualSset appsv1.StatefulSet,
) error {
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
			storageCmp := k8s.CompareStorageRequests(pvc.Spec.Resources, expectedClaim.Spec.Resources)
			if !storageCmp.Increase {
				// not an increase, nothing to do
				continue
			}

			newSize := expectedClaim.Spec.Resources.Requests.Storage()
			log.Info("Resizing PVC storage requests. Depending on the volume provisioner, "+
				"Pods may need to be manually deleted for the filesystem to be resized.",
				"namespace", pvc.Namespace, "es_name", es.Name, "pvc_name", pvc.Name,
				"old_value", pvc.Spec.Resources.Requests.Storage().String(), "new_value", newSize.String())

			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = *newSize
			if err := k8sClient.Update(&pvc); err != nil {
				return err
			}
		}
	}
	return nil
}

// annotateForRecreation stores the StatefulSet spec with updated storage requirements
// in an annotation of the Elasticsearch resource, to be recreated at the next reconciliation.
func annotateForRecreation(
	k8sClient k8s.Client,
	es esv1.Elasticsearch,
	actualSset appsv1.StatefulSet,
	expectedClaims []corev1.PersistentVolumeClaim,
) error {
	log.Info("Preparing StatefulSet re-creation to account for PVC resize",
		"namespace", es.Namespace, "es_name", es.Name, "statefulset_name", actualSset.Name)

	actualSset.Spec.VolumeClaimTemplates = expectedClaims
	asJSON, err := json.Marshal(actualSset)
	if err != nil {
		return err
	}
	if es.Annotations == nil {
		es.Annotations = make(map[string]string, 1)
	}
	es.Annotations[RecreateStatefulSetAnnotationPrefix+actualSset.Name] = string(asJSON)

	return k8sClient.Update(&es)
}

// needsRecreate returns true if the StatefulSet needs to be re-created to account for volume expansion.
func needsRecreate(expectedSset appsv1.StatefulSet, actualSset appsv1.StatefulSet) bool {
	for _, expectedClaim := range expectedSset.Spec.VolumeClaimTemplates {
		actualClaim := sset.GetClaim(actualSset.Spec.VolumeClaimTemplates, expectedClaim.Name)
		if actualClaim == nil {
			continue
		}
		storageCmp := k8s.CompareStorageRequests(actualClaim.Spec.Resources, expectedClaim.Spec.Resources)
		if storageCmp.Increase {
			return true
		}
	}
	return false
}

// recreateStatefulSets re-creates StatefulSets as specified in Elasticsearch annotations, to account for
// resized volume claims.
// This function acts as a state machine that depends on the annotation and the UID of existing StatefulSets.
// A standard flow may span over multiple reconciliations like this:
// 1. No annotation set: nothing to do.
// 2. An annotation specifies StatefulSet Foo needs to be recreated. That StatefulSet actually exists: delete it.
// 3. An annotation specifies StatefulSet Foo needs to be recreated. That StatefulSet does not exist: create it.
// 4. An annotation specifies StatefulSet Foo needs to be recreated. That StatefulSet actually exists, but with
//    a different UID: the re-creation is over, remove the annotation.
func recreateStatefulSets(k8sClient k8s.Client, es esv1.Elasticsearch) (int, error) {
	toRecreate, err := ssetsToRecreate(es)
	if err != nil {
		return 0, err
	}
	recreations := len(toRecreate)

	for annotation, toRecreate := range toRecreate {
		var existing appsv1.StatefulSet
		err := k8sClient.Get(k8s.ExtractNamespacedName(&toRecreate), &existing)
		switch {
		// error case
		case err != nil && !apierrors.IsNotFound(err):
			return recreations, err

		// already exists with the same UID: deletion case
		case existing.UID == toRecreate.UID:
			log.Info("Deleting StatefulSet to account for resized PVCs, it will be recreated automatically",
				"namespace", es.Namespace, "es_name", es.Name, "statefulset_name", existing.Name)
			// mark the Pod as owned by the ES resource while the StatefulSet is removed
			if err := updatePodOwners(k8sClient, es, existing); err != nil {
				return recreations, err
			}
			if err := deleteStatefulSet(k8sClient, existing); err != nil {
				return recreations, err
			}

		// already deleted: creation case
		case err != nil && apierrors.IsNotFound(err):
			log.Info("Re-creating StatefulSet to account for resized PVCs",
				"namespace", es.Namespace, "es_name", es.Name, "statefulset_name", toRecreate.Name)
			if err := recreateStatefulSet(k8sClient, toRecreate); err != nil {
				return recreations, err
			}

		// already recreated (existing.UID != toRecreate.UID): we're done
		default:
			// remove the temporary pod owner set before the StatefulSet was deleted
			if err := removeESPodOwner(k8sClient, es, existing); err != nil {
				return recreations, err
			}
			// remove the annotation
			delete(es.Annotations, annotation)
			if err := k8sClient.Update(&es); err != nil {
				return recreations, err
			}
			// one less recreation
			recreations--
		}
	}

	return recreations, nil
}

// ssetsToRecreate returns the list of StatefulSet that should be recreated, based on annotations
// in the Elasticsearch resource.
func ssetsToRecreate(es esv1.Elasticsearch) (map[string]appsv1.StatefulSet, error) {
	toRecreate := map[string]appsv1.StatefulSet{}
	for key, value := range es.Annotations {
		if !strings.HasPrefix(key, RecreateStatefulSetAnnotationPrefix) {
			continue
		}
		var sset appsv1.StatefulSet
		if err := json.Unmarshal([]byte(value), &sset); err != nil {
			return nil, err
		}
		toRecreate[key] = sset
	}
	return toRecreate, nil
}

func deleteStatefulSet(k8sClient k8s.Client, sset appsv1.StatefulSet) error {
	opts := client.DeleteOptions{}
	// ensure we are not deleting the StatefulSet that was already recreated with a different UID
	opts.Preconditions = &metav1.Preconditions{UID: &sset.UID}
	// ensure Pods are not also deleted
	orphanPolicy := metav1.DeletePropagationOrphan
	opts.PropagationPolicy = &orphanPolicy

	return k8sClient.Delete(&sset, &opts)
}

func recreateStatefulSet(k8sClient k8s.Client, sset appsv1.StatefulSet) error {
	// don't keep metadata inherited from the old StatefulSet
	newObjMeta := metav1.ObjectMeta{
		Name:            sset.Name,
		Namespace:       sset.Namespace,
		Labels:          sset.Labels,
		Annotations:     sset.Annotations,
		OwnerReferences: sset.OwnerReferences,
		Finalizers:      sset.Finalizers,
	}
	sset.ObjectMeta = newObjMeta
	return k8sClient.Create(&sset)
}

// updatePodOwners marks all Pods managed by the given StatefulSet as owned by the Elasticsearch resource.
// Pods are already owned by the StatefulSet resource, but when we'll (temporarily) delete that StatefulSet
// they won't be owned anymore. At this point if the Elasticsearch resource is deleted (before the StatefulSet
// is re-created), we also want the Pods to be deleted automatically.
func updatePodOwners(k8sClient k8s.Client, es esv1.Elasticsearch, statefulSet appsv1.StatefulSet) error {
	log.V(1).Info("Setting an owner ref to the Elasticsearch resource on the future orphan Pods",
		"namespace", es.Namespace, "es_name", es.Name, "statefulset_name", statefulSet.Name)
	return updatePods(k8sClient, statefulSet, func(p *corev1.Pod) error {
		return controllerutil.SetOwnerReference(&es, p, scheme.Scheme)
	})
}

// removeESPodOwner removes any reference to the ES resource from the Pods, that was set in updatePodOwners.
func removeESPodOwner(k8sClient k8s.Client, es esv1.Elasticsearch, statefulSet appsv1.StatefulSet) error {
	log.V(1).Info("Removing any Pod owner ref set to the Elasticsearch resource after StatefulSet re-creation",
		"namespace", es.Namespace, "es_name", es.Name, "statefulset_name", statefulSet.Name)
	updateFunc := func(p *corev1.Pod) error {
		for i, ownerRef := range p.OwnerReferences {
			if ownerRef.UID == es.UID && ownerRef.Name == es.Name && ownerRef.Kind == es.Kind {
				// remove from the owner ref slice
				p.OwnerReferences = append(p.OwnerReferences[:i], p.OwnerReferences[i+1:]...)
				return nil
			}
		}
		return nil
	}
	return updatePods(k8sClient, statefulSet, updateFunc)
}

// updatePods applies updateFunc on all existing Pods from the StatefulSet, then update those Pods.
func updatePods(k8sClient k8s.Client, statefulSet appsv1.StatefulSet, updateFunc func(p *corev1.Pod) error) error {
	pods, err := sset.GetActualPodsForStatefulSet(k8sClient, k8s.ExtractNamespacedName(&statefulSet))
	if err != nil {
		return err
	}
	for i := range pods {
		if err := updateFunc(&pods[i]); err != nil {
			return err
		}
		if err := k8sClient.Update(&pods[i]); err != nil {
			return err
		}
	}
	return nil
}
