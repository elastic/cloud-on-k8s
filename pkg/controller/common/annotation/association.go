// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// CurrAssocStatusAnnotation describes the currently observed association status of an object.
	CurrAssocStatusAnnotation = "association.k8s.elastic.co/current-status"
	// PrevAssocStatusAnnotation describes the previously observed association status of an object.
	PrevAssocStatusAnnotation = "association.k8s.elastic.co/previous-status"
)

// ForAssociationStatusChange constructs the annotation map for an association status change event.
func ForAssociationStatusChange(prevStatus, currStatus commonv1.AssociationStatusMap) (map[string]string, error) {
	return map[string]string{
		PrevAssocStatusAnnotation: prevStatus.String(),
		CurrAssocStatusAnnotation: currStatus.String(),
	}, nil
}

// ExtractAssociationStatusStrings extracts the association status strings from the provided meta object.
func ExtractAssociationStatusStrings(obj metav1.ObjectMeta) (prevStatus, currStatus string) {
	if obj.Annotations == nil {
		return "", ""
	}

	prevStatus = obj.Annotations[PrevAssocStatusAnnotation]
	currStatus = obj.Annotations[CurrAssocStatusAnnotation]
	return
}
