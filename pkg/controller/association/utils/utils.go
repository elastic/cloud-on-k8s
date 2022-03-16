// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package utils

import (
	"encoding/json"
	"reflect"
	"unsafe"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)


func SimpleAssociationConf(assoc commonv1.Association, assocConf *commonv1.AssociationConf) *commonv1.AssociationConf  {
	if assocConf == nil {
		return setAssocConfFromAnnotation(assoc)
	}
	return assocConf
}

func MultipleAssociationConf(assoc commonv1.Association, ref types.NamespacedName, assocConfs map[types.NamespacedName]commonv1.AssociationConf ) *commonv1.AssociationConf  {
	if len(assocConfs) == 0 {
		return setAssocConfFromAnnotation(assoc)
	}
	assocConf, found := assocConfs[ref]
	if !found {
		return setAssocConfFromAnnotation(assoc)
	}
	return &assocConf
}

// GetAssociationConf extracts the association configuration from the given object by reading the annotations.
/*func GetAssociationConf(association commonv1.Association) (*commonv1.AssociationConf, error) {
	accessor := meta.NewAccessor()
	annotations, err := accessor.Annotations(association)
	if err != nil {
		return nil, err
	}
	return extractAssocConfFromAnnotation(annotations, association.AssociationConfAnnotationName())
}*/

// setAssocConfFromAnnotation sets the association configuration extracted from the annotations in the given association.
func setAssocConfFromAnnotation(assoc commonv1.Association) *commonv1.AssociationConf {
	assocConf, err := extractAssocConfFromAnnotation(assoc.Associated().GetAnnotations(), assoc.AssociationConfAnnotationName())
	if err != nil {
		// ignore this unlikely unexpected error that should not happen as we control the annotation
		return nil
	}
	assoc.SetAssociationConf(assocConf)
	return assocConf
}

// ExtractAssocConfFromAnnotation extracts the association configuration from annotations and an annotation name.
func extractAssocConfFromAnnotation(annotations map[string]string, annotationName string) (*commonv1.AssociationConf, error) {
	if len(annotations) == 0 {
		return nil, nil
	}

	var assocConf commonv1.AssociationConf
	serializedConf, exists := annotations[annotationName]
	if !exists || serializedConf == "" {
		return nil, nil
	}

	if err := json.Unmarshal(unsafeStringToBytes(serializedConf), &assocConf); err != nil {
		return nil, errors.Wrapf(err, "failed to extract association configuration")
	}

	return &assocConf, nil
}

func unsafeStringToBytes(s string) []byte {
	hdr := *(*reflect.StringHeader)(unsafe.Pointer(&s))    //nolint:govet
	return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{ //nolint:govet
		Data: hdr.Data,
		Len:  hdr.Len,
		Cap:  hdr.Len,
	}))
}