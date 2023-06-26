// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reconciler

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

// labels set on secrets which cannot rely on owner references due to https://github.com/kubernetes/kubernetes/issues/65200,
// but should still be garbage-collected (best-effort) by the operator upon owner deletion.
const (
	SoftOwnerNamespaceLabel = "eck.k8s.elastic.co/owner-namespace"
	SoftOwnerNameLabel      = "eck.k8s.elastic.co/owner-name"
	SoftOwnerKindLabel      = "eck.k8s.elastic.co/owner-kind"
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
		client.HasLabels{SoftOwnerNamespaceLabel, SoftOwnerNameLabel, SoftOwnerKindLabel},
	); err != nil {
		return err
	}
	// remove any secret whose owner doesn't exist
	for i := range secrets.Items {
		secret := secrets.Items[i]
		softOwner, referenced := SoftOwnerRefFromLabels(secret.Labels)
		if !referenced {
			continue
		}
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
				options := client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &secret.UID}}
				if err := c.Delete(ctx, &secret, &options); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				continue
			}
			return err
		}
		// owner still exists, keep the secret
	}
	return nil
}

// restrictedToOwnerNamespace returns true if a resource should have its owner in the same namespace, based on the kind of owner.
func restrictedToOwnerNamespace(kind string) bool {
	return kind != policyv1alpha1.Kind
}
