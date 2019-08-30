// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	// "github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	ifs "github.com/elastic/cloud-on-k8s/pkg/controller/common/interfaces"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// CurrAssocStatusAnnotation describes the currently observed association status of an object.
	CurrAssocStatusAnnotation = "ifs.k8s.elastic.co/current-status"
	// PrevAssocStatusAnnotation describes the previously observed association status of an object.
	PrevAssocStatusAnnotation = "ifs.k8s.elastic.co/previous-status"
)

// ForAssociationStatusChange constructs the annotation map for an association status change event.
func ForAssociationStatusChange(prevStatus, currStatus ifs.AssociationStatus) map[string]string {
	return map[string]string{
		CurrAssocStatusAnnotation: string(currStatus),
		PrevAssocStatusAnnotation: string(prevStatus),
	}
}

// ExtractAssociationStatus extracts the association status values from the provided meta object.
func ExtractAssociationStatus(obj metav1.ObjectMeta) (prevStatus, currStatus ifs.AssociationStatus) {
	if obj.Annotations == nil {
		return ifs.AssociationUnknown, ifs.AssociationUnknown
	}

	prevStatus = ifs.AssociationStatus(obj.Annotations[PrevAssocStatusAnnotation])
	currStatus = ifs.AssociationStatus(obj.Annotations[CurrAssocStatusAnnotation])
	return
}
