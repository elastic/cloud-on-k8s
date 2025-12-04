// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
)

// DoesPolicyMatchObject checks if the given StackConfigPolicy targets the given object (e.g., Elasticsearch or Kibana).
// A policy targets an object if both following conditions are met:
// 1. The policy is in either the operator namespace or the same namespace as the object
// 2. The policy's label selector matches the object's labels
// Returns true if the policy targets the object, false otherwise, and an error if the label selector is invalid.
func DoesPolicyMatchObject(policy *policyv1alpha1.StackConfigPolicy, obj metav1.Object, operatorNamespace string) (bool, error) {
	// Check namespace restrictions; the policy must be in operator namespace or same namespace as the target object.
	// This enforces the scoping rules: policies in the operator namespace are global,
	// policies in other namespaces can only target resources in their own namespace.
	if policy.Namespace != operatorNamespace && policy.Namespace != obj.GetNamespace() {
		return false, nil
	}

	// Convert the label selector from the policy spec into a labels.Selector that can be used for matching
	selector, err := metav1.LabelSelectorAsSelector(&policy.Spec.ResourceSelector)
	if err != nil {
		// Return error if the label selector syntax is invalid (e.g., malformed expressions)
		return false, err
	}

	// Check if the label selector matches the object's labels.
	// This is the actual matching logic - does this policy's selector match this object's labels?
	if !selector.Matches(labels.Set(obj.GetLabels())) {
		return false, nil
	}

	// Both conditions met: namespace is valid and labels match
	return true, nil
}
