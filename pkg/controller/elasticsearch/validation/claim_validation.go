// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// noIllegalVolumeClaimDeletePolicyChange validates that the user is not changing VolumeClaimDeletePolicy on an existing
// Elasticsearch cluster. This is implemented as a create validation and not as an update validation to allow usage from
// within the controller as well where we don't have the context of the previous version of the spec anymore.
// But we can infer a policy change by checking the ownerReferences on the PVCs in the knowledge that a Retain policy
// must mean no owner reference on the PVCs and vice versa for the Remove* policies
func noIllegalVolumeClaimDeletePolicyChange(c k8s.Client, es esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	sets, err := sset.RetrieveActualStatefulSets(c, k8s.ExtractNamespacedName(&es))
	if err != nil {
		// interal error when fetching ssets should not lead to a failing validation.
		return errs
	}
	// OK no ssets probably a new cluster
	if len(sets) == 0 {
		return errs
	}

	var isOwnedByES bool
	for _, sset := range sets {
		for _, pvc := range sset.Spec.VolumeClaimTemplates {
			if k8s.HasOwner(&pvc, &es) {
				isOwnedByES = true
				break
			}
		}
	}

	policy := es.Spec.VolumeClaimDeletePolicyOrDefault()
	for _, forbidden := range []bool{
		policy == esv1.RetainPolicy && isOwnedByES,
		policy != esv1.RetainPolicy && !isOwnedByES,
	} {
		if forbidden {
			errs = append(errs, field.Forbidden(field.NewPath("spec").Child("volumeClaimDeletePolicy"), forbiddenPolicyChgMsg))
		}
	}

	return errs
}
