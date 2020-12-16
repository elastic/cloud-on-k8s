// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

const (
	// CurrAssocStatusAnnotation describes the currently observed association status of an object.
	CurrAssocStatusAnnotation = "association.k8s.elastic.co/current-status"
	// PrevAssocStatusAnnotation describes the previously observed association status of an object.
	PrevAssocStatusAnnotation = "association.k8s.elastic.co/previous-status"
)

// ForAssociationStatusChange constructs the annotation map for an association status change event.
func ForAssociationStatusChange(prevStatus, currStatus commonv1.AssociationStatusMap) (map[string]string, error) {
	prev, err := json.Marshal(prevStatus)
	if err != nil {
		return nil, err
	}
	curr, err := json.Marshal(currStatus)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		PrevAssocStatusAnnotation: string(prev),
		CurrAssocStatusAnnotation: string(curr),
	}, nil
}

// ExtractAssociationStatus extracts the association status values from the provided meta object.
func ExtractAssociationStatus(obj metav1.ObjectMeta) (prevStatus, currStatus commonv1.AssociationStatus, err error) {
	if obj.Annotations == nil {
		return commonv1.AssociationUnknown, commonv1.AssociationUnknown, nil
	}

	prev := &commonv1.AssociationStatusMap{}
	if err = json.Unmarshal([]byte(obj.Annotations[PrevAssocStatusAnnotation]), prev); err != nil {
		return
	}

	curr := &commonv1.AssociationStatusMap{}
	if err = json.Unmarshal([]byte(obj.Annotations[CurrAssocStatusAnnotation]), curr); err != nil {
		return
	}

	prevStatus = prev.Aggregate()
	currStatus = curr.Aggregate()
	return
}
