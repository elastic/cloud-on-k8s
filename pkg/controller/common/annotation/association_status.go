// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// CurrAssocStatusAnnotation describes the currently observed association status of an object.
	CurrAssocStatusAnnotation = "association.k8s.elastic.co/current-status"
	// PrevAssocStatusAnnotation describes the previously observed association status of an object.
	PrevAssocStatusAnnotation = "association.k8s.elastic.co/previous-status"
)

// ForAssociationStatusChange constructs the annotation map for an association status change event.
func ForAssociationStatusChange(prevStatus, currStatus commonv1alpha1.AssociationStatus) map[string]string {
	return map[string]string{
		CurrAssocStatusAnnotation: string(currStatus),
		PrevAssocStatusAnnotation: string(prevStatus),
	}
}

// ExtractAssociationStatus extracts the association status values from the provided meta object.
func ExtractAssociationStatus(obj metav1.ObjectMeta) (prevStatus, currStatus commonv1alpha1.AssociationStatus) {
	if obj.Annotations == nil {
		return commonv1alpha1.AssociationUnknown, commonv1alpha1.AssociationUnknown
	}

	prevStatus = commonv1alpha1.AssociationStatus(obj.Annotations[PrevAssocStatusAnnotation])
	currStatus = commonv1alpha1.AssociationStatus(obj.Annotations[CurrAssocStatusAnnotation])
	return
}
