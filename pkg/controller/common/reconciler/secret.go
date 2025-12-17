// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reconciler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

// labels set on secrets which cannot rely on owner references due to https://github.com/kubernetes/kubernetes/issues/65200,
// but should still be garbage-collected (best-effort) by the operator upon owner deletion.
const (
	SoftOwnerNamespaceLabel = "eck.k8s.elastic.co/owner-namespace"
	SoftOwnerNameLabel      = "eck.k8s.elastic.co/owner-name"
	SoftOwnerKindLabel      = "eck.k8s.elastic.co/owner-kind"
	SoftOwnerRefsAnnotation = "eck.k8s.elastic.co/owner-refs"
)

func WithPostUpdate(f func()) func(p *Params) {
	return func(p *Params) {
		p.PostUpdate = f
	}
}

// ReconcileSecret creates or updates the actual secret to match the expected one.
// Existing annotations or labels that are not expected are preserved.
func ReconcileSecret(ctx context.Context, c k8s.Client, expected corev1.Secret, owner client.Object, opts ...func(*Params)) (corev1.Secret, error) {
	var reconciled corev1.Secret

	params := Params{
		Context:    ctx,
		Client:     c,
		Owner:      owner,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			// update if expected labels and annotations are not there
			return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
				!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
				// or if secret data is not strictly equal
				!reflect.DeepEqual(expected.Data, reconciled.Data)
		},
		UpdateReconciled: func() {
			// set expected annotations and labels, but don't remove existing ones
			// that may have been defaulted or set by the user on the existing resource
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
			reconciled.Data = expected.Data
		},
	}
	for _, opt := range opts {
		opt(&params)
	}
	if err := ReconcileResource(params); err != nil {
		return corev1.Secret{}, err
	}
	return reconciled, nil
}

type SoftOwnerRef struct {
	Namespace string
	Name      string
	Kind      string
}

// SoftOwnerRefFromLabels parses the given labels to return a SoftOwnerRef.
// It also returns a boolean indicating whether a soft owner was referenced.
func SoftOwnerRefFromLabels(labels map[string]string) (SoftOwnerRef, bool) {
	if len(labels) == 0 {
		return SoftOwnerRef{}, false
	}
	namespace := labels[SoftOwnerNamespaceLabel]
	name := labels[SoftOwnerNameLabel]
	kind := labels[SoftOwnerKindLabel]
	if namespace == "" || name == "" || kind == "" {
		return SoftOwnerRef{}, false
	}
	return SoftOwnerRef{Namespace: namespace, Name: name, Kind: kind}, true
}

// SetMultipleSoftOwners sets multiple soft owner references to an object.
// Unlike single ownership (which uses labels), multiple ownership stores a JSON-encoded
// list of owner references in the SoftOwnerRefsAnnotation annotation to accommodate multiple owners.
//
// The function sets:
//   - The label SoftOwnerKindLabel indicating the soft owner kind (e.g., "StackConfigPolicy")
//   - The annotation SoftOwnerRefsAnnotation containing a JSON list of all owner namespaced names
//   - Removes any existing single-owner labels if present
//
// Returns an error if JSON marshaling fails.
func SetMultipleSoftOwners(obj metav1.Object, ownerKind string, owners []types.NamespacedName) error {
	objLabels := obj.GetLabels()
	if objLabels == nil {
		objLabels = map[string]string{}
	} else {
		// Remove single owner labels if they exist
		delete(objLabels, SoftOwnerNamespaceLabel)
		delete(objLabels, SoftOwnerNameLabel)
	}

	// Mark this Secret as being soft-owned by ownerKind
	objLabels[SoftOwnerKindLabel] = ownerKind

	objAnnotations := obj.GetAnnotations()
	if objAnnotations == nil {
		objAnnotations = map[string]string{}
	}

	// Build a set of owner references using namespaced names as keys.
	ownerRefs := sets.Set[string]{}
	for _, o := range owners {
		ownerRefs.Insert(o.String())
	}
	// Store the owner references as a JSON-encoded annotation
	ownerRefsBytes, err := json.Marshal(sets.List(ownerRefs))
	if err != nil {
		return err
	}

	objAnnotations[SoftOwnerRefsAnnotation] = string(ownerRefsBytes)
	obj.SetAnnotations(objAnnotations)
	obj.SetLabels(objLabels)
	return nil
}

// SetSingleSoftOwner sets a single soft owner reference to an object.
// This uses labels (SoftOwnerKindLabel, SoftOwnerNameLabel, SoftOwnerNamespaceLabel)
// to store the ownership relationship, allowing the owner to manage
// the object's lifecycle without using Kubernetes OwnerReferences.
// Removes any existing multi-owner annotation if present.
func SetSingleSoftOwner(obj metav1.Object, owner SoftOwnerRef) {
	objLabels := obj.GetLabels()
	if objLabels == nil {
		objLabels = map[string]string{}
	}

	objAnnotations := obj.GetAnnotations()
	if objAnnotations != nil {
		// Remove multi-owner annotation if it exists
		delete(objAnnotations, SoftOwnerRefsAnnotation)
	}

	objLabels[SoftOwnerNamespaceLabel] = owner.Namespace
	objLabels[SoftOwnerNameLabel] = owner.Name
	objLabels[SoftOwnerKindLabel] = owner.Kind
	obj.SetLabels(objLabels)
	obj.SetAnnotations(objAnnotations)
}

// RemoveSoftOwner removes a soft owner from an object.
// It handles both single-owner (label-based) and multi-owner (annotation-based) scenarios.
//
// For single-owner objects:
//   - If the owner matches, removes all soft owner labels
//   - If the owner doesn't match, leaves the object unchanged
//
// For multi-owner objects:
//   - Removes the owner from the JSON list in the SoftOwnerRefsAnnotation
//   - Updates the annotation with the remaining owners or removes it if no owners remain
//
// Returns the number of remaining owners after removal and an error if there's a problem
// with JSON marshalling/unmarshalling.
func RemoveSoftOwner(obj metav1.Object, owner types.NamespacedName) (remainingCount int, err error) {
	objLabels := obj.GetLabels()
	if objLabels == nil {
		return 0, nil
	}

	objAnnotations := obj.GetAnnotations()

	// Check for multi-owner ownership (annotation-based)
	if ownerRefsBytes, exists := objAnnotations[SoftOwnerRefsAnnotation]; exists {
		// Multi-owner soft owned object - parse and update the set
		var ownerRefsSlice []string
		if err := json.Unmarshal([]byte(ownerRefsBytes), &ownerRefsSlice); err != nil {
			return 0, err
		}

		ownerRefs := sets.New(ownerRefsSlice...)
		// Remove the specified owner from the set
		ownerRefs.Delete(owner.String())
		if ownerRefs.Len() == 0 {
			// No owners remain, remove the annotation
			delete(objAnnotations, SoftOwnerRefsAnnotation)
			return 0, nil
		}

		// Marshal the updated owner list back to JSON
		ownerRefsBytes, err := json.Marshal(sets.List(ownerRefs))
		if err != nil {
			return 0, err
		}

		// Update the annotation with the new owner list
		objAnnotations[SoftOwnerRefsAnnotation] = string(ownerRefsBytes)
		return len(ownerRefs), nil
	}

	// Handle single-owner ownership (label-based)
	currentOwner, referenced := SoftOwnerRefFromLabels(objLabels)
	if !referenced {
		// No soft owner found
		return 0, nil
	}

	// Check if the single owner matches the owner to be removed
	if currentOwner.Name == owner.Name && currentOwner.Namespace == owner.Namespace {
		// Remove the soft owner labels since this was the only owner
		delete(objLabels, SoftOwnerNamespaceLabel)
		delete(objLabels, SoftOwnerNameLabel)
		return 0, nil
	}

	// The owner to remove doesn't match the current owner, so no change
	return 1, nil
}

// IsSoftOwnedBy checks if an object is soft-owned by the given owner.
// It handles both single-owner (label-based) and multi-owner (annotation-based) scenarios.
// Returns true if the object is owned by the specified owner, false otherwise, and an error
// if there's a problem unmarshalling the owner references from annotations.
func IsSoftOwnedBy(obj metav1.Object, ownerKind string, owner types.NamespacedName) (bool, error) {
	objLabels := obj.GetLabels()
	if objLabels == nil {
		return false, nil
	}

	if objOwnerKind := objLabels[SoftOwnerKindLabel]; objOwnerKind != ownerKind {
		return false, nil
	}

	objAnnotations := obj.GetAnnotations()
	// Check for multi-owner ownership (annotation-based)
	if ownerRefsBytes, exists := objAnnotations[SoftOwnerRefsAnnotation]; exists {
		var ownerRefsSlice []string
		if err := json.Unmarshal([]byte(ownerRefsBytes), &ownerRefsSlice); err != nil {
			return false, err
		}

		ownerRefs := sets.New(ownerRefsSlice...)
		return ownerRefs.Has(owner.String()), nil
	}

	// Fall back to single-owner ownership (label-based)
	currentOwner, referenced := SoftOwnerRefFromLabels(objLabels)
	if !referenced {
		// No soft owner found in labels
		return false, nil
	}

	// Check if the single owner matches the given owner
	return currentOwner.Name == owner.Name && currentOwner.Namespace == owner.Namespace, nil
}

// SoftOwnerRefs returns the soft owner references of the given object.
func SoftOwnerRefs(obj metav1.Object) ([]SoftOwnerRef, error) {
	// Check if this object has a soft-owner kind label set
	ownerKind, exists := obj.GetLabels()[SoftOwnerKindLabel]
	if !exists {
		// Not a soft-owned object
		return nil, nil
	}

	// Check for multi-owner ownership (annotation-based)
	if ownerRefsBytes, exists := obj.GetAnnotations()[SoftOwnerRefsAnnotation]; exists {
		// Multi-owner soft owned object - parse the list of owners
		var ownerRefsSlice []string
		if err := json.Unmarshal([]byte(ownerRefsBytes), &ownerRefsSlice); err != nil {
			return nil, err
		}

		// Convert the list to []SoftOwnerRef
		var ownerRefsNsn []SoftOwnerRef
		for _, nsnStr := range ownerRefsSlice {
			// Split the string format "namespace/name" into components
			nsnComponents := strings.Split(nsnStr, string(types.Separator))
			if len(nsnComponents) != 2 {
				// Skip malformed entries
				continue
			}
			ownerRefsNsn = append(ownerRefsNsn, SoftOwnerRef{Namespace: nsnComponents[0], Name: nsnComponents[1], Kind: ownerKind})
		}

		return ownerRefsNsn, nil
	}

	// Fall back to single-owner ownership (label-based)
	currentOwner, referenced := SoftOwnerRefFromLabels(obj.GetLabels())
	if !referenced {
		// No soft owner found in labels
		return nil, nil
	}

	// Return the single owner as a slice with one element
	return []SoftOwnerRef{currentOwner}, nil
}

// ReconcileSecretNoOwnerRef should be called to reconcile a Secret for which we explicitly don't want
// an owner reference to be set, and want existing ownerReferences from previous operator versions to be removed,
// because of this k8s bug: https://github.com/kubernetes/kubernetes/issues/65200 (fixed in k8s 1.20).
//
// It makes sense to use this function for secrets which are likely to be manually
// copied into other namespaces by the end user.
// Because of the k8s bug mentioned above, the ownerReference could trigger a racy garbage collection
// that deletes all child resources, potentially resulting in data loss.
// See https://github.com/elastic/cloud-on-k8s/issues/3986 for more details.
//
// Since they won't have an ownerReference specified, reconciled secrets will not be deleted automatically on parent deletion.
// To account for that, we add labels to reference the "soft owner", for garbage collection by the operator on parent resource deletion.
func ReconcileSecretNoOwnerRef(ctx context.Context, c k8s.Client, expected corev1.Secret, softOwner runtime.Object) (corev1.Secret, error) {
	// this function is similar to "ReconcileSecret", but:
	// - we don't pass an owner
	// - we remove the existing owner
	// - we set additional labels to perform garbage collection on owner deletion (best-effort)
	ownerMeta, err := meta.Accessor(softOwner)
	if err != nil {
		return corev1.Secret{}, err
	}

	// don't mutate expected (no side effects), make a copy
	expected = *expected.DeepCopy()
	if expected.Labels == nil {
		expected.Labels = make(map[string]string)
	}
	expected.Labels[SoftOwnerNamespaceLabel] = ownerMeta.GetNamespace()
	expected.Labels[SoftOwnerNameLabel] = ownerMeta.GetName()
	expected.Labels[SoftOwnerKindLabel] = softOwner.GetObjectKind().GroupVersionKind().Kind

	var reconciled corev1.Secret
	if err := ReconcileResource(Params{
		Context:    ctx,
		Client:     c,
		Owner:      nil,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			// update if expected labels and annotations are not there
			return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
				!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
				// or if secret data is not strictly equal
				!reflect.DeepEqual(expected.Data, reconciled.Data) ||
				// or if an existing owner should be removed
				k8s.HasOwner(&reconciled, ownerMeta)
		},
		UpdateReconciled: func() {
			// set expected annotations and labels, but don't remove existing ones
			// that may have been defaulted or set by the user on the existing resource
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
			reconciled.Data = expected.Data
			// remove existing owner
			k8s.RemoveOwner(&reconciled, ownerMeta)
		},
	}); err != nil {
		return corev1.Secret{}, err
	}
	return reconciled, nil
}

// GarbageCollectSoftOwnedSecrets deletes all secrets whose labels reference a soft owner.
// To be called once that owner gets deleted.
func GarbageCollectSoftOwnedSecrets(ctx context.Context, c k8s.Client, deletedOwner types.NamespacedName, ownerKind string) error {
	log := ulog.FromContext(ctx)
	var secrets corev1.SecretList
	// restrict to secrets on which we set the soft owner labels
	listOpts := []client.ListOption{client.MatchingLabels{
		SoftOwnerNamespaceLabel: deletedOwner.Namespace,
		SoftOwnerNameLabel:      deletedOwner.Name,
		SoftOwnerKindLabel:      ownerKind,
	}}
	// restrict to secrets in the parent namespace, we don't want to delete secrets users may have manually copied into
	// other namespaces (except for kind where we control these secrets)
	if restrictedToOwnerNamespace(ownerKind) {
		listOpts = append(listOpts, client.InNamespace(deletedOwner.Namespace))
	}
	if err := c.List(ctx, &secrets, listOpts...); err != nil {
		return err
	}
	for i := range secrets.Items {
		s := secrets.Items[i]
		log.Info("Garbage collecting secret",
			"namespace", deletedOwner.Namespace, "secret_name", s.Name,
			"owner_name", deletedOwner.Name, "owner_kind", ownerKind)
		err := c.Delete(ctx, &s, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &s.UID}})
		if apierrors.IsNotFound(err) {
			// already deleted, all good
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// GarbageCollectAllSoftOwnedOrphanSecrets iterates over all Secrets that reference a soft owner. If the owner
// doesn't exist anymore, it deletes the secrets.
// Should be called on operator startup, after cache warm-up, to cover cases where
// the operator is down when the owner is deleted.
// If the operator is up, garbage collection is already handled by GarbageCollectSoftOwnedSecrets on owner deletion.
func GarbageCollectAllSoftOwnedOrphanSecrets(ctx context.Context, c k8s.Client, ownerKinds map[string]client.Object) error {
	// retrieve all secrets that reference a soft owner
	var secrets corev1.SecretList
	if err := c.List(ctx,
		&secrets,
		client.HasLabels{SoftOwnerKindLabel},
	); err != nil {
		return err
	}
	// remove any secret whose owner doesn't exist
	for i := range secrets.Items {
		secret := secrets.Items[i]
		softOwners, err := SoftOwnerRefs(&secret)
		if err != nil {
			return err
		}
		if len(softOwners) == 0 {
			continue
		}

		missingOwners := make(map[types.NamespacedName]client.Object)
		for _, softOwner := range softOwners {
			if restrictedToOwnerNamespace(softOwner.Kind) && softOwner.Namespace != secret.Namespace {
				// Secret references an owner in a different namespace: this likely results
				// from a "manual" copy of the secret in another namespace, not handled by the operator.
				// We don't want to touch that secret.
				continue
			}
			owner, managed := ownerKinds[softOwner.Kind]
			if !managed {
				continue
			}
			owner = k8s.DeepCopyObject(owner)
			err := c.Get(ctx, types.NamespacedName{Namespace: softOwner.Namespace, Name: softOwner.Name}, owner)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// owner doesn't exit anymore
					ulog.FromContext(ctx).Info("Deleting secret as part of garbage collection",
						"namespace", secret.Namespace, "secret_name", secret.Name,
						"owner_kind", softOwner.Kind, "owner_namespace", softOwner.Namespace, "owner_name", softOwner.Name,
					)
					missingOwners[types.NamespacedName{Namespace: softOwner.Namespace, Name: softOwner.Name}] = owner
					continue
				}
				return err
			}
		}

		if len(missingOwners) == len(softOwners) {
			options := client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &secret.UID}}
			if err := c.Delete(ctx, &secret, &options); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
		// owner still exists, keep the secret
	}
	return nil
}

// restrictedToOwnerNamespace returns true if a resource should have its owner in the same namespace, based on the kind of owner.
func restrictedToOwnerNamespace(kind string) bool {
	return kind != policyv1alpha1.Kind
}
