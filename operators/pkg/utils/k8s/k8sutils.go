// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// ToObjectMeta returns an ObjectMeta based on the given NamespacedName
func ToObjectMeta(namespacedName types.NamespacedName) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Namespace: namespacedName.Namespace,
		Name:      namespacedName.Name,
	}
}

// ExtractNamespacedName returns an NamespacedName based on the given ObjectMeta
func ExtractNamespacedName(object metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: object.GetNamespace(),
		Name:      object.GetName(),
	}
}

func GetKind(s *runtime.Scheme, obj runtime.Object) (string, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	// if the object referenced is actually persisted, we can just get kind from meta
	// if we are building an object reference to something not yet persisted, we fallback to scheme
	kind := gvk.Kind
	if len(kind) == 0 {
		gvks, _, err := s.ObjectKinds(obj)
		if err != nil {
			return "", err
		}
		kind = gvks[0].Kind
	}
	return kind, nil
}
