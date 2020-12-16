// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// noIllegalVolumeClaimDeletePolicyChange validates that the user is not changing VolumeClaimDeletePolicy on an existing
// Elasticsearch cluster. This is implemented as a create validation and not as an update validation to allow usage from
// within the controller as well where we don't have the context of the previous version of the spec anymore.
// But we can infer a policy change by checking the ownerReferences on the PVCs in the knowledge that a Retain policy
// must mean no owner reference on the PVCs and vice versa for the Remove* policies
func noIllegalVolumeClaimDeletePolicyChange(k8sClient k8s.Client, es esv1.Elasticsearch) field.ErrorList {
	var errs field.ErrorList
	matchingLabels := label.NewLabelSelectorForElasticsearch(es)
	var pvcs v1.PersistentVolumeClaimList
	if err := k8sClient.List(&pvcs, matchingLabels); err != nil {
		// fail or continue here? let's not fail admission here for now, but if we don't
		return errs
	}

	// OK no PVCs probably a new cluster
	if len(pvcs.Items) == 0 {
		return errs
	}

	var isOwnedByES bool
	for _, pvc := range pvcs.Items {
		if k8s.HasOwner(&pvc, &es) {
			isOwnedByES = true
			break
		}
	}

	policy := es.Spec.VolumeClaimDeletePolicyOrDefault()
	// this can be simplified ...
	if policy == esv1.RetainPolicy && isOwnedByES {
		errs = append(errs, field.Forbidden(field.NewPath("spec").Child("volumeClaimDeletePolicy"), forbiddenPolicyChgMsg))
	}
	if policy != esv1.RetainPolicy && !isOwnedByES {
		errs = append(errs, field.Forbidden(field.NewPath("spec").Child("volumeClaimDeletePolicy"), forbiddenPolicyChgMsg))
	}

	return errs
}
