// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"encoding/json"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/sset"

	volumevalidations "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume/validations"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	// RecreateStatefulSetAnnotationPrefix is used to annotate the Logstash resource
	// with StatefulSets to recreate. The StatefulSet name is appended to this name.
	RecreateStatefulSetAnnotationPrefix = "logstash.k8s.elastic.co/recreate-"
)

// handleVolumeExpansion works around the immutability of VolumeClaimTemplates in StatefulSets by:
// 1. Validating that the volume expansion request is valid (storage requests can only increase)
// 1. updating storage requests in PVCs whose storage class supports volume expansion
// 3. Setting an annotation on the Logstash resource with details of the statefulset to be recreated.
// It returns a boolean indicating whether the StatefulSet needs to be recreated.
// Note that some storage drivers also require Pods to be deleted/recreated for the filesystem to be resized
// (as opposed to a hot resize while the Pod is running). This is left to the responsibility of the user.
// This should be handled differently once supported by the StatefulSet controller: https://github.com/kubernetes/kubernetes/issues/68737.
func handleVolumeExpansion(
	ctx context.Context,
	k8sClient k8s.Client,
	ls lsv1alpha1.Logstash,
	expectedSset appsv1.StatefulSet,
	actualSset appsv1.StatefulSet,
	validateStorageClass bool,
) (bool, error) {
	// ensure there are no incompatible storage size modification
	if err := volumevalidations.ValidateClaimsStorageUpdate(
		ctx,
		k8sClient,
		actualSset.Spec.VolumeClaimTemplates,
		expectedSset.Spec.VolumeClaimTemplates,
		validateStorageClass); err != nil {
		return false, err
	}
	// resize all PVCs that can be resized
	err := resizePVCs(ctx, k8sClient, ls, expectedSset, actualSset)

	if err != nil {
		return false, err
	}

	//// schedule the StatefulSet for recreation if needed
	recreatesets, err := ssetsToRecreate(ls)
	if err != nil {
		return false, err
	}

	_, present := recreatesets[RecreateStatefulSetAnnotationPrefix+actualSset.Name]

	if present {
		ulog.FromContext(ctx).V(1).Info("Skipping annotation, already added")
		return true, nil
	}

	if needsRecreate(expectedSset, actualSset) {
		ulog.FromContext(ctx).Info("handleVolumeExpansion: Needs Recreate")
		return true, annotateForRecreation(ctx, k8sClient, ls, actualSset, expectedSset.Spec.VolumeClaimTemplates)
	}

	return false, nil
}

// resizePVCs updates the spec of all existing PVCs whose storage requests can be expanded,
// according to their storage class and what's specified in the expected claim.
// It returns an error if the requested storage size is incompatible with the PVC.
func resizePVCs(
	ctx context.Context,
	k8sClient k8s.Client,
	ls lsv1alpha1.Logstash,
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
			pvc := pvc
			storageCmp := k8s.CompareStorageRequests(pvc.Spec.Resources, expectedClaim.Spec.Resources)
			if !storageCmp.Increase {
				// not an increase, nothing to do
				continue
			}

			newSize := expectedClaim.Spec.Resources.Requests.Storage()
			ulog.FromContext(ctx).Info("Resizing PVC storage requests. Depending on the volume provisioner, "+
				"Pods may need to be manually deleted for the filesystem to be resized.",
				"namespace", pvc.Namespace, "ls_name", ls.Name, "pvc_name", pvc.Name,
				"old_value", pvc.Spec.Resources.Requests.Storage().String(), "new_value", newSize.String())

			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = *newSize
			if err := k8sClient.Update(ctx, &pvc); err != nil {
				return err
			}
		}
	}
	return nil
}

// annotateForRecreation stores the StatefulSet spec with updated storage requirements
// in an annotation of the Logstash resource, to be recreated at the next reconciliation.
func annotateForRecreation(
	ctx context.Context,
	k8sClient k8s.Client,
	ls lsv1alpha1.Logstash,
	actualSset appsv1.StatefulSet,
	expectedClaims []corev1.PersistentVolumeClaim,
) error {
	ulog.FromContext(ctx).Info("annotate for recreation: Preparing StatefulSet re-creation to account for PVC resize",
		"namespace", ls.Namespace, "ls_name", ls.Name, "statefulset_name", actualSset.Name)

	actualSset.Spec.VolumeClaimTemplates = expectedClaims
	asJSON, err := json.Marshal(actualSset)
	if err != nil {
		return err
	}
	if ls.Annotations == nil {
		ls.Annotations = make(map[string]string, 1)
	}
	ls.Annotations[RecreateStatefulSetAnnotationPrefix+actualSset.Name] = string(asJSON)
	return k8sClient.Update(ctx, &ls)
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

// recreateStatefulSets re-creates StatefulSets as specified in Logstash annotations, to account for
// resized volume claims.
// This function acts as a state machine that depends on the annotation and the UID of existing StatefulSets.
// A standard flow may span over multiple reconciliations like this:
//  1. No annotation set: nothing to do.
//  2. An annotation specifies StatefulSet Foo needs to be recreated. That StatefulSet actually exists: delete it.
//  3. An annotation specifies StatefulSet Foo needs to be recreated. That StatefulSet does not exist: create it.
//  4. An annotation specifies StatefulSet Foo needs to be recreated. That StatefulSet actually exists, but with
//     a different UID: the re-creation is over, remove the annotation.
func recreateStatefulSets(ctx context.Context, k8sClient k8s.Client, ls lsv1alpha1.Logstash) (int, error) {
	log := ulog.FromContext(ctx)
	recreateList, err := ssetsToRecreate(ls)

	if err != nil {
		return 0, err
	}
	recreations := len(recreateList)
	log.V(1).Info("Recreating Stateful Sets", "recreations count", recreations)

	for annotation, toRecreate := range recreateList {
		toRecreate := toRecreate
		var existing appsv1.StatefulSet
		err := k8sClient.Get(ctx, k8s.ExtractNamespacedName(&toRecreate), &existing)

		switch {
		// error case
		case err != nil && !apierrors.IsNotFound(err):
			log.V(1).Info("StatefulSet not found")
			return recreations, err

		// already exists with the same UID: deletion case
		case existing.UID == toRecreate.UID:
			log.Info("Deleting StatefulSet to account for resized PVCs, it will be recreated automatically",
				"namespace", ls.Namespace, "ls_name", ls.Name, "statefulset_name", existing.Name)
			// mark the Pod as owned by the LS resource while the StatefulSet is removed
			if err := updatePodOwners(ctx, k8sClient, ls, existing); err != nil {
				return recreations, err
			}
			if err := deleteStatefulSet(ctx, k8sClient, existing); err != nil {
				return recreations, err
			}

		// already deleted: creation case
		case err != nil && apierrors.IsNotFound(err):
			log.Info("Recreating StatefulSet to account for resized PVCs",
				"namespace", ls.Namespace, "ls_name", ls.Name, "statefulset_name", toRecreate.Name)
			if err := recreateStatefulSet(ctx, k8sClient, toRecreate); err != nil {
				return recreations, err
			}

		// already recreated (existing.UID != toRecreate.UID): we're done
		default:
			// remove the temporary pod owner set before the StatefulSet was deleted
			if err := removeLSPodOwner(ctx, k8sClient, ls, existing); err != nil {
				return recreations, err
			}
			// remove the annotation
			delete(ls.Annotations, annotation)
			if err := k8sClient.Update(ctx, &ls); err != nil {
				return recreations, err
			}
			// one less recreation
			recreations--
		}
	}
	return recreations, nil
}

// ssetsToRecreate returns the list of StatefulSet that should be recreated, based on annotations
// in the Logstash resource.
func ssetsToRecreate(ls lsv1alpha1.Logstash) (map[string]appsv1.StatefulSet, error) {
	toRecreate := map[string]appsv1.StatefulSet{}
	for key, value := range ls.Annotations {
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

func deleteStatefulSet(ctx context.Context, k8sClient k8s.Client, sset appsv1.StatefulSet) error {
	opts := client.DeleteOptions{}
	// ensure we are not deleting the StatefulSet that was already recreated with a different UID
	opts.Preconditions = &metav1.Preconditions{UID: &sset.UID}
	// ensure Pods are not also deleted
	orphanPolicy := metav1.DeletePropagationOrphan
	opts.PropagationPolicy = &orphanPolicy
	log := ulog.FromContext(ctx)
	log.V(1).Info("Deleting old stateful set", "ss_name", sset.Name)
	return k8sClient.Delete(ctx, &sset, &opts)
}

func recreateStatefulSet(ctx context.Context, k8sClient k8s.Client, sset appsv1.StatefulSet) error {
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
	log := ulog.FromContext(ctx)
	log.Info("Recreating stateful set", "ss_name", sset.Name)
	return k8sClient.Create(ctx, &sset)
}

// updatePodOwners marks all Pods managed by the given StatefulSet as owned by the Logstash resource.
// Pods are already owned by the StatefulSet resource, but when we'll (temporarily) delete that StatefulSet
// they won't be owned anymore. At this point if the Elasticsearch resource is deleted (before the StatefulSet
// is re-created), we also want the Pods to be deleted automatically.
func updatePodOwners(ctx context.Context, k8sClient k8s.Client, ls lsv1alpha1.Logstash, statefulSet appsv1.StatefulSet) error {
	ulog.FromContext(ctx).V(1).Info("Setting an owner ref to the Logstash resource on the future orphan Pods",
		"namespace", ls.Namespace, "ls_name", ls.Name, "statefulset_name", statefulSet.Name)
	return updatePods(ctx, k8sClient, statefulSet, func(p *corev1.Pod) error {
		return controllerutil.SetOwnerReference(&ls, p, scheme.Scheme)
	})
}

// removeLSPodOwner removes any reference to the Logstash resource from the Pods, that was set in updatePodOwners.
func removeLSPodOwner(ctx context.Context, k8sClient k8s.Client, ls lsv1alpha1.Logstash, statefulSet appsv1.StatefulSet) error {
	ulog.FromContext(ctx).V(1).Info("Removing any Pod owner ref set to the Logstash resource after StatefulSet re-creation",
		"namespace", ls.Namespace, "ls_name", ls.Name, "statefulset_name", statefulSet.Name)
	updateFunc := func(p *corev1.Pod) error {
		for i, ownerRef := range p.OwnerReferences {
			if ownerRef.UID == ls.UID && ownerRef.Name == ls.Name && ownerRef.Kind == ls.Kind {
				// remove from the owner ref slice
				p.OwnerReferences = append(p.OwnerReferences[:i], p.OwnerReferences[i+1:]...)
				return nil
			}
		}
		return nil
	}
	return updatePods(ctx, k8sClient, statefulSet, updateFunc)
}

// updatePods applies updateFunc on all existing Pods from the StatefulSet, then update those Pods.
func updatePods(ctx context.Context, k8sClient k8s.Client, statefulSet appsv1.StatefulSet, updateFunc func(p *corev1.Pod) error) error {
	pods, err := sset.GetActualPodsForStatefulSet(k8sClient, k8s.ExtractNamespacedName(&statefulSet))
	if err != nil {
		return err
	}
	for i := range pods {
		if err := updateFunc(&pods[i]); err != nil {
			return err
		}
		if err := k8sClient.Update(ctx, &pods[i]); err != nil {
			return err
		}
	}
	return nil
}
