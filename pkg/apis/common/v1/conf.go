// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"encoding/json"
	"unsafe"

	"github.com/pkg/errors"
)

// GetAndSetAssociationConf returns the association configuration if it is not nil, else the association configured is
// read from the annotation and put back in the given association.
func GetAndSetAssociationConf(assoc Association, assocConf *AssociationConf) (*AssociationConf, error) {
	if assocConf == nil {
		return setAssocConfFromAnnotation(assoc)
	}
	return assocConf, nil
}

// GetAndSetAssociationConfByRef returns the association configuration corresponding to the namespaced name of the
// referenced resource if it is found in the given map of association configurations.
// The association configurations map is not persisted and can be cleared by an update of the parent resource
// (see https://github.com/elastic/cloud-on-k8s/issues/4709#issuecomment-1042898108), hence we check if this map is empty,
// in which case we try to populate it again from the annotation.
func GetAndSetAssociationConfByRef(assoc Association, ref ObjectSelector, assocConfs map[ObjectSelector]AssociationConf) (*AssociationConf, error) {
	assocConf, found := assocConfs[ref]
	if !found {
		return setAssocConfFromAnnotation(assoc)
	}
	return &assocConf, nil
}

// setAssocConfFromAnnotation sets the association configuration extracted from the annotations in the given association.
func setAssocConfFromAnnotation(assoc Association) (*AssociationConf, error) {
	assocConf, err := extractAssocConfFromAnnotation(assoc.Associated().GetAnnotations(), assoc.AssociationConfAnnotationName())
	if err != nil {
		return nil, err
	}
	assoc.SetAssociationConf(assocConf)
	return assocConf, nil
}

// extractAssocConfFromAnnotation extracts the association configuration from annotations and an annotation name.
func extractAssocConfFromAnnotation(annotations map[string]string, annotationName string) (*AssociationConf, error) {
	var assocConf AssociationConf
	serializedConf, exists := annotations[annotationName]
	if !exists || serializedConf == "" {
		return nil, nil
	}

	if err := json.Unmarshal(unsafeStringToBytes(serializedConf), &assocConf); err != nil {
		return nil, errors.Wrapf(err, "failed to extract association configuration")
	}

	return &assocConf, nil
}

// unsafeStringToBytes converts a string to a byte array without making extra allocations.
// since we read potentially large strings from annotations on every reconcile loop, this should help
// reduce the amount of garbage created.
func unsafeStringToBytes(s string) []byte {
	if s == "" {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
