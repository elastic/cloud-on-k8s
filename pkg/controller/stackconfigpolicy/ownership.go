// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// setSingleSoftOwner marks a Secret as soft-owned by a single StackConfigPolicy.
// This uses labels (reconciler.SoftOwnerKindLabel, reconciler.SoftOwnerNameLabel, reconciler.SoftOwnerNamespaceLabel)
// to store the ownership relationship, allowing the policy to manage
// the Secret's lifecycle without using Kubernetes OwnerReferences.
func setSingleSoftOwner(secret *corev1.Secret, policy policyv1alpha1.StackConfigPolicy) {
	if secret == nil {
		return
	}

	if secret.Annotations != nil {
		// Remove multi-owner annotation if it exists
		delete(secret.Annotations, commonannotation.SoftOwnerRefsAnnotation)
	}

	filesettings.SetSoftOwner(secret, policy)
}

// setMultipleSoftOwners marks a Secret as soft-owned by multiple StackConfigPolicies.
// Unlike single ownership (which uses labels), multiple ownership stores a JSON-encoded
// map of owner references in annotations to accommodate multiple policies.
//
// The function sets:
//   - The label reconciler.SoftOwnerKindLabel indicating the soft owner kind to policyv1alpha1.Kind
//   - The annotation commonannotation.SoftOwnerRefsAnnotation containing a JSON map of all owner namespaced names
//
// Returns an error if JSON marshaling fails.
func setMultipleSoftOwners(secret *corev1.Secret, policies []policyv1alpha1.StackConfigPolicy) error {
	if secret == nil {
		return nil
	}

	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	} else {
		// Remove single owner labels if they exist
		delete(secret.Labels, reconciler.SoftOwnerNamespaceLabel)
		delete(secret.Labels, reconciler.SoftOwnerNameLabel)
	}

	// Mark this Secret as being soft-owned by StackConfigPolicy resources
	secret.Labels[reconciler.SoftOwnerKindLabel] = policyv1alpha1.Kind

	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}

	// Build a map of owner references using namespaced names as keys.
	// We use struct{} as values since we only care about the keys (acts as a set).
	ownerRefs := make(map[string]struct{})
	for _, p := range policies {
		ownerRefs[k8s.ExtractNamespacedName(&p).String()] = struct{}{}
	}

	// Store the owner references as a JSON-encoded annotation
	ownerRefsBytes, err := json.Marshal(ownerRefs)
	if err != nil {
		return err
	}

	secret.Annotations[commonannotation.SoftOwnerRefsAnnotation] = string(ownerRefsBytes)
	return nil
}

// isPolicySoftOwner checks if the given StackConfigPolicy is a soft owner of the Secret.
// It handles both single-owner (reconciler.SoftOwnerNameLabel, reconciler.SoftOwnerNamespaceLabel)
// and multi-owner (commonannotation.SoftOwnerRefsAnnotation) scenarios.
// Returns true or false depending on whether the policy is an owner of the secret
// and an error if there's a problem unmarshalling the owner references
func isPolicySoftOwner(secret *corev1.Secret, policyNsn types.NamespacedName) (bool, error) {
	if secret == nil {
		return false, nil
	}

	// Check if this Secret is soft-owned by a StackConfigPolicy
	if ownerKind := secret.Labels[reconciler.SoftOwnerKindLabel]; ownerKind != policyv1alpha1.Kind {
		// Not a policy soft-owned secret
		return false, nil
	}

	// Check for multi-policy ownership (annotation-based)
	if ownerRefsBytes, exists := secret.Annotations[commonannotation.SoftOwnerRefsAnnotation]; exists {
		// Multi-policy soft owned secret - parse the JSON map of owners
		var ownerRefs map[string]struct{}
		if err := json.Unmarshal([]byte(ownerRefsBytes), &ownerRefs); err != nil {
			return false, err
		}
		// Check if the given policy is in the set of owners
		_, exists := ownerRefs[types.NamespacedName{Name: policyNsn.Name, Namespace: policyNsn.Namespace}.String()]
		return exists, nil
	}

	// Fall back to single-policy ownership (label-based)
	currentOwner, referenced := reconciler.SoftOwnerRefFromLabels(secret.Labels)
	if !referenced {
		// No soft owner found in labels
		return false, nil
	}

	// Check if the single owner matches the given policy
	return currentOwner.Name == policyNsn.Name && currentOwner.Namespace == policyNsn.Namespace, nil
}

// removePolicySoftOwner removes a StackConfigPolicy if it is soft owning the given secret.
// It handles both single-owner (reconciler.SoftOwnerNameLabel, reconciler.SoftOwnerNamespaceLabel)
// and multi-owner (commonannotation.SoftOwnerRefsAnnotation) scenarios.
//
// For single-owner secrets:
//   - If the owner matches, removes all soft owner labels/annotations
//   - If the owner doesn't match, leaves the secret unchanged
//
// For multi-owner secrets:
//   - Removes the policy from the JSON map in annotations
//   - Updates the annotation with the remaining owners or removes the annotation if no owners remain
//
// Returns the number of remaining owners after removal and an error if there's a problem with JSON marshaling/unmarshalling
func removePolicySoftOwner(secret *corev1.Secret, policyNsn types.NamespacedName) (int, error) {
	if secret == nil {
		return 0, nil
	}

	// Check for multi-policy ownership (annotation-based)
	if ownerRefsBytes, exists := secret.Annotations[commonannotation.SoftOwnerRefsAnnotation]; exists {
		// Multi-policy soft owned secret - parse and update the owner map
		var ownerRefs map[string]struct{}
		if err := json.Unmarshal([]byte(ownerRefsBytes), &ownerRefs); err != nil {
			return 0, err
		}

		// Remove the specified policy from the owner map
		delete(ownerRefs, types.NamespacedName{Name: policyNsn.Name, Namespace: policyNsn.Namespace}.String())
		if len(ownerRefs) == 0 {
			// No owners remain, remove the annotation
			delete(secret.Annotations, commonannotation.SoftOwnerRefsAnnotation)
			return 0, nil
		}

		// Marshal the updated owner map back to JSON
		ownerRefsBytes, err := json.Marshal(ownerRefs)
		if err != nil {
			return 0, err
		}

		// Update the annotation with the new owner list
		secret.Annotations[commonannotation.SoftOwnerRefsAnnotation] = string(ownerRefsBytes)
		return len(ownerRefs), nil
	}

	// Handle single-policy ownership (label-based)
	currentOwner, referenced := reconciler.SoftOwnerRefFromLabels(secret.Labels)
	if !referenced {
		// No soft owner found
		return 0, nil
	}

	// Check if the single owner matches the policy to be removed
	if currentOwner.Name == policyNsn.Name && currentOwner.Namespace == policyNsn.Namespace {
		// Remove the soft owner labels since this was the only owner
		delete(secret.Labels, reconciler.SoftOwnerNamespaceLabel)
		delete(secret.Labels, reconciler.SoftOwnerNameLabel)
		return 0, nil
	}

	// The policy to remove doesn't match the current owner, so no change
	return 1, nil
}
