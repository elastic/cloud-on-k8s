// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package v1 provides a fake ECK-module type that lives outside pkg/apis/.
// It is used by ssacrlint tests to verify that the -cr-path-pattern check
// requires pkg/apis/ in the path: a type in the right module root but the
// wrong subtree must not be flagged.
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// OtherType satisfies client.Object but lives under other/, not pkg/apis/.
// The linter must not flag Update or Patch calls on this type.
type OtherType struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

// DeepCopyObject satisfies runtime.Object.
func (o *OtherType) DeepCopyObject() runtime.Object {
	copy := *o
	copy.ObjectMeta = *o.ObjectMeta.DeepCopy()
	return &copy
}
