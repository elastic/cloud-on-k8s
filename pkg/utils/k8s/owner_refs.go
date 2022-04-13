// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package k8s

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// OverrideControllerReference overrides the controller owner reference with the given owner reference.
func OverrideControllerReference(obj metav1.Object, newOwner metav1.OwnerReference) {
	owners := obj.GetOwnerReferences()

	ref := indexOfCtrlRef(owners)
	if ref == -1 {
		obj.SetOwnerReferences([]metav1.OwnerReference{newOwner})
		return
	}
	owners[ref] = newOwner
	obj.SetOwnerReferences(owners)
}

func HasOwner(resource, owner metav1.Object) bool {
	if owner == nil || resource == nil {
		return false
	}
	found, _ := FindOwner(resource, owner)
	return found
}

func RemoveOwner(resource, owner metav1.Object) {
	if resource == nil || owner == nil {
		return
	}
	found, index := FindOwner(resource, owner)
	if !found {
		return
	}
	owners := resource.GetOwnerReferences()
	// remove the owner at index i from the slice
	owners = append(owners[:index], owners[index+1:]...)
	resource.SetOwnerReferences(owners)
}

func FindOwner(resource, owner metav1.Object) (found bool, index int) {
	if owner == nil || resource == nil {
		return false, 0
	}
	ownerRefs := resource.GetOwnerReferences()
	for i := range ownerRefs {
		if ownerRefs[i].Name == owner.GetName() && ownerRefs[i].UID == owner.GetUID() {
			return true, i
		}
	}
	return false, 0
}
