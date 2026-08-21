// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package v1 provides minimal fake ECK CRD types for ssacrdlint tests.
// The analyzer is configured in tests to treat this package's prefix as the
// ECK API prefix, allowing test cases to use real controller-runtime types
// without importing the full ECK module.
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// FakeCRD is a stand-in for an ECK custom resource (e.g. Elasticsearch).
// It embeds TypeMeta and ObjectMeta so it satisfies client.Object, mirroring
// what real ECK CRDs do.
type FakeCRD struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

// DeepCopyObject satisfies runtime.Object.
func (f *FakeCRD) DeepCopyObject() runtime.Object {
	copy := *f
	copy.ObjectMeta = *f.ObjectMeta.DeepCopy()
	return &copy
}
