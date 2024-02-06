// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	volumevalidations "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume/validations"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// handleVolumeExpansion works around the immutability of VolumeClaimTemplates in StatefulSets by:
// 1. updating storage requests in PVCs whose storage class supports volume expansion
// 2. scheduling the StatefulSet for recreation with the new storage spec
// It returns a boolean indicating whether the StatefulSet needs to be recreated.
// Note that some storage drivers also require Pods to be deleted/recreated for the filesystem to be resized
// (as opposed to a hot resize while the Pod is running). This is left to the responsibility of the user.
// This should be handled differently once supported by the StatefulSet controller: https://github.com/kubernetes/kubernetes/issues/68737.
func HandleVolumeExpansion(
	ctx context.Context,
	k8sClient k8s.Client,
	owner client.Object,
	ownerKind string,
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
	err := resizePVCs(ctx, k8sClient, owner, expectedSset, actualSset)
	if err != nil {
		return false, err
	}

	// schedule the StatefulSet for recreation if needed
	if needsRecreate(expectedSset, actualSset) {
		return true, annotateForRecreation(ctx, k8sClient, owner, ownerKind, actualSset, expectedSset.Spec.VolumeClaimTemplates)
	}

	return false, nil
}

// ResizePVCs updates the spec of all existing PVCs whose storage requests can be expanded,
// according to their storage class and what's specified in the expected claim.
// It returns an error if the requested storage size is incompatible with the PVC.
func resizePVCs(
	ctx context.Context,
	k8sClient k8s.Client,
	owner client.Object,
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
			accessor := meta.NewAccessor()
			name, _ := accessor.Name(owner)

			newSize := expectedClaim.Spec.Resources.Requests.Storage()
			ulog.FromContext(ctx).Info("Resizing PVC storage requests. Depending on the volume provisioner, "+
				"Pods may need to be manually deleted for the filesystem to be resized.",
				"namespace", pvc.Namespace, "name", name, "pvc_name", pvc.Name,
				"old_value", pvc.Spec.Resources.Requests.Storage().String(), "new_value", newSize.String())

			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = *newSize
			if err := k8sClient.Update(ctx, &pvc); err != nil {
				return err
			}
		}
	}
	return nil
}

// AnnotateForRecreation stores the StatefulSet spec with updated storage requirements
// in an annotation of the owning resource, to be recreated at the next reconciliation.
func annotateForRecreation(
	ctx context.Context,
	k8sClient k8s.Client,
	owner client.Object,
	ownerKind string,
	actualSset appsv1.StatefulSet,
	expectedClaims []corev1.PersistentVolumeClaim,
) error {
	namespacedName := namespacedNameFromObject(owner)

	ulog.FromContext(ctx).Info("Preparing StatefulSet re-creation to account for PVC resize",
		"namespace", namespacedName.Namespace, "name", namespacedName.Name, "statefulset_name", actualSset.Name)

	actualSset.Spec.VolumeClaimTemplates = expectedClaims
	asJSON, err := json.Marshal(actualSset)

	if err != nil {
		return err
	}

	err = setAnnotation(owner, getRecreateStatefulSetAnnotationKey(ownerKind, actualSset.Name), string(asJSON))
	if err != nil {
		return err
	}

	return k8sClient.Update(ctx, owner)
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

// RecreateStatefulSets re-creates StatefulSets as specified in annotations, to account for
// resized volume claims.
// This function acts as a state machine that depends on the annotation and the UID of existing StatefulSets.
// A standard flow may span over multiple reconciliations like this:
//  1. No annotation set: nothing to do.
//  2. An annotation specifies StatefulSet Foo needs to be recreated. That StatefulSet actually exists: delete it.
//  3. An annotation specifies StatefulSet Foo needs to be recreated. That StatefulSet does not exist: create it.
//  4. An annotation specifies StatefulSet Foo needs to be recreated. That StatefulSet actually exists, but with
//     a different UID: the re-creation is over, remove the annotation.
func RecreateStatefulSets(ctx context.Context, k8sClient k8s.Client, owner client.Object, ownerKind string) (int, error) {
	log := ulog.FromContext(ctx)
	recreateList, err := ssetsToRecreate(owner, ownerKind)
	if err != nil {
		return 0, err
	}
	recreations := len(recreateList)

	for annotation, toRecreate := range recreateList {
		toRecreate := toRecreate

		namespacedName := namespacedNameFromObject(owner)

		var existing appsv1.StatefulSet
		err = k8sClient.Get(ctx, k8s.ExtractNamespacedName(&toRecreate), &existing)
		switch {
		// error case
		case err != nil && !apierrors.IsNotFound(err):
			return recreations, err

		// already exists with the same UID: deletion case
		case existing.UID == toRecreate.UID && !apierrors.IsNotFound(err):

			log.Info("Deleting StatefulSet to account for resized PVCs, it will be recreated automatically",
				"namespace", namespacedName.Namespace, "name", namespacedName.Name, "statefulset_name", existing.Name)
			// mark the Pod as owned by the component resource while the StatefulSet is removed
			if err := updatePodOwners(ctx, k8sClient, owner, ownerKind, existing); err != nil {
				return recreations, err
			}
			if err := deleteStatefulSet(ctx, k8sClient, existing); err != nil {
				if apierrors.IsNotFound(err) {
					return recreations, nil
				}
				return recreations, err
			}

		// already deleted: creation case
		case err != nil && apierrors.IsNotFound(err):
			log.Info("Re-creating StatefulSet to account for resized PVCs",
				"namespace", namespacedName.Namespace, "name", namespacedName.Name, "statefulset_name", toRecreate.Name)
			if err := createStatefulSet(ctx, k8sClient, toRecreate); err != nil {
				return recreations, err
			}

		// already recreated (existing.UID != toRecreate.UID): we're done
		default:
			// remove the temporary pod owner set before the StatefulSet was deleted
			if err := removePodOwner(ctx, k8sClient, owner, ownerKind, existing); err != nil {
				return recreations, err
			}
			// remove the annotation
			err := deleteAnnotation(owner, annotation)
			if err != nil {
				return recreations, err
			}

			if err := k8sClient.Update(ctx, owner); err != nil {
				return recreations, err
			}
			// one less recreation
			recreations--
		}
	}

	return recreations, nil
}

func deleteStatefulSet(ctx context.Context, k8sClient k8s.Client, sset appsv1.StatefulSet) error {
	opts := client.DeleteOptions{}
	// ensure we are not deleting the StatefulSet that was already recreated with a different UID
	opts.Preconditions = &metav1.Preconditions{UID: &sset.UID}
	// ensure Pods are not also deleted
	orphanPolicy := metav1.DeletePropagationOrphan
	opts.PropagationPolicy = &orphanPolicy
	ulog.FromContext(ctx).V(1).Info("Deleting stateful set", "statefulset_name", sset.Name, "namespace", sset.Namespace)

	return k8sClient.Delete(ctx, &sset, &opts)
}

func createStatefulSet(ctx context.Context, k8sClient k8s.Client, sset appsv1.StatefulSet) error {
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
	return k8sClient.Create(ctx, &sset)
}

// updatePodOwners marks all Pods managed by the given StatefulSet as owned by the parent resource.
// Pods are already owned by the StatefulSet resource, but when we'll (temporarily) delete that StatefulSet
// they won't be owned anymore. At this point if the resource is deleted (before the StatefulSet
// is re-created), we also want the Pods to be deleted automatically.
func updatePodOwners(ctx context.Context, k8sClient k8s.Client, owner client.Object, ownerKind string, statefulSet appsv1.StatefulSet) error {
	namespacedName := namespacedNameFromObject(owner)

	ulog.FromContext(ctx).V(1).Info("Setting an owner ref to the component resource on the future orphan Pods",
		"namespace", namespacedName.Namespace, "name", namespacedName.Name, "statefulset_name", statefulSet.Name)
	return updatePods(ctx, k8sClient, getStatefulSetLabelName(ownerKind), statefulSet, func(p *corev1.Pod) error {
		return controllerutil.SetOwnerReference(owner, p, scheme.Scheme)
	})
}

// removePodOwner removes any reference to the resource from the Pods, that was set in updatePodOwners.
func removePodOwner(ctx context.Context, k8sClient k8s.Client, owner client.Object, ownerKind string, statefulSet appsv1.StatefulSet) error {
	accessor := meta.NewAccessor()
	name, _ := accessor.Name(owner)
	namespace, _ := accessor.Namespace(owner)
	UID, err := accessor.UID(owner)

	if err != nil {
		return err
	}

	ulog.FromContext(ctx).V(1).Info("Removing any Pod owner ref set to the component resource after StatefulSet re-creation",
		"namespace", namespace, "name", name, "statefulset_name", statefulSet.Name)
	updateFunc := func(p *corev1.Pod) error {
		for i, ownerRef := range p.OwnerReferences {
			if ownerRef.UID == UID && ownerRef.Name == name && ownerRef.Kind == ownerKind {
				// remove from the owner ref slice
				p.OwnerReferences = append(p.OwnerReferences[:i], p.OwnerReferences[i+1:]...)
				return nil
			}
		}
		return nil
	}
	return updatePods(ctx, k8sClient, getStatefulSetLabelName(ownerKind), statefulSet, updateFunc)
}

// updatePods applies updateFunc on all existing Pods from the StatefulSet, then update those Pods.
func updatePods(ctx context.Context, k8sClient k8s.Client, label string, statefulSet appsv1.StatefulSet, updateFunc func(p *corev1.Pod) error) error {
	pods, err := sset.GetActualPodsForStatefulSet(k8sClient, k8s.ExtractNamespacedName(&statefulSet), label)

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

func namespacedNameFromObject(owner client.Object) types.NamespacedName {
	accessor := meta.NewAccessor()
	name, err := accessor.Name(owner)
	if err != nil {
		name = "-"
	}

	namespace, _ := accessor.Namespace(owner)

	if err != nil {
		namespace = "-"
	}
	return types.NamespacedName{Name: name, Namespace: namespace}
}

func setAnnotation(owner client.Object, annotationKey string, annotationValue string) error {
	accessor := meta.NewAccessor()
	annotations, err := accessor.Annotations(owner)

	if err != nil {
		return err
	}
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	annotations[annotationKey] = annotationValue
	err = accessor.SetAnnotations(owner, annotations)
	if err != nil {
		return err
	}
	return nil
}

func deleteAnnotation(owner client.Object, annotation string) error {
	accessor := meta.NewAccessor()
	annotations, err := accessor.Annotations(owner)

	if err != nil {
		return err
	}

	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	delete(annotations, annotation)
	err = accessor.SetAnnotations(owner, annotations)

	if err != nil {
		return err
	}
	return nil
}

// ssetsToRecreate returns the list of StatefulSet that should be recreated, based on annotations
// in the parent component resource.
func ssetsToRecreate(owner client.Object, ownerKind string) (map[string]appsv1.StatefulSet, error) {
	accessor := meta.NewAccessor()
	annotations, err := accessor.Annotations(owner)
	if err != nil {
		return nil, err
	}

	annotationPrefix := getRecreateStatefulSetAnnotationPrefix(ownerKind)

	toRecreate := map[string]appsv1.StatefulSet{}
	for key, value := range annotations {
		if !strings.HasPrefix(key, annotationPrefix) {
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

func getStatefulSetLabelName(ownerKind string) string {
	return fmt.Sprintf("%s.k8s.elastic.co/statefulset-name", strings.ToLower(ownerKind))
}

func getRecreateStatefulSetAnnotationPrefix(ownerKind string) string {
	return fmt.Sprintf("%s.k8s.elastic.co/recreate-", strings.ToLower(ownerKind))
}

func getRecreateStatefulSetAnnotationKey(ownerKind string, ssetName string) string {
	return fmt.Sprintf("%s%s", getRecreateStatefulSetAnnotationPrefix(ownerKind), ssetName)
}
