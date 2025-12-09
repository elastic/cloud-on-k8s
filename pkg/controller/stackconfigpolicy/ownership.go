// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// setSingleSoftOwner marks a Secret as soft-owned by a single StackConfigPolicy.
func setSingleSoftOwner(secret *corev1.Secret, policy policyv1alpha1.StackConfigPolicy) {
	if secret == nil {
		return
	}

	reconciler.SetSingleSoftOwner(secret, reconciler.SoftOwnerRef{
		Namespace: policy.GetNamespace(),
		Name:      policy.GetName(),
		Kind:      policyv1alpha1.Kind,
	})
}

// setMultipleSoftOwners marks a Secret as soft-owned by multiple StackConfigPolicies.
func setMultipleSoftOwners(secret *corev1.Secret, policies []policyv1alpha1.StackConfigPolicy) error {
	if secret == nil {
		return nil
	}
	ownersNsn := make([]types.NamespacedName, len(policies))
	for idx, p := range policies {
		ownersNsn[idx] = k8s.ExtractNamespacedName(&p)
	}
	return reconciler.SetMultipleSoftOwners(secret, policyv1alpha1.Kind, ownersNsn)
}

// isPolicySoftOwner checks if the given StackConfigPolicy is a soft owner of the Secret.
func isPolicySoftOwner(secret *corev1.Secret, policyNsn types.NamespacedName) (bool, error) {
	if secret == nil {
		return false, nil
	}
	return reconciler.IsSoftOwnedBy(secret, policyv1alpha1.Kind, policyNsn)
}

// removePolicySoftOwner removes a StackConfigPolicy from the soft owners of the given secret.
// Returns the number of remaining owners after removal.
func removePolicySoftOwner(secret *corev1.Secret, policyNsn types.NamespacedName) (int, error) {
	if secret == nil {
		return 0, nil
	}

	return reconciler.RemoveSoftOwner(secret, policyNsn)
}
