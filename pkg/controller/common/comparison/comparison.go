// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package comparison

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Equal checks that two objects are equal, ignoring the TypeMeta and ResourceVersion. Often used for tests ensuring that we receive structs that match what we expect without
// runtime-specific information
func Equal(a, b runtime.Object) bool {
	typemeta := cmpopts.IgnoreTypes(metav1.TypeMeta{})
	rv := cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")
	return cmp.Equal(a, b, typemeta, rv)
}

// Diff returns the difference between two objects ignoring the TypeMeta and ResourceVersion. Often used for tests ensuring that we receive structs that match what we expect without
// runtime-specific information
func Diff(a, b runtime.Object) string {
	typemeta := cmpopts.IgnoreTypes(metav1.TypeMeta{})
	rv := cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")
	timestamps := cmpopts.IgnoreTypes(metav1.Time{})
	return cmp.Diff(a, b, typemeta, rv, timestamps)
}

// AssertEqual errors if two objects ignoring the TypeMeta and ResourceVersion. Equivalent to calling t.Error()
func AssertEqual(t *testing.T, a, b runtime.Object) {
	t.Helper()
	if diff := Diff(a, b); diff != "" {
		t.Errorf("Expected objects to be the same. Differences:\n%v", diff)
	}
}

// RequireEqual errors if two objects ignoring the TypeMeta and ResourceVersion. Equivalent to calling t.Fatal()
func RequireEqual(t *testing.T, a, b runtime.Object) {
	t.Helper()
	if diff := Diff(a, b); diff != "" {
		t.Fatalf("Expected objects to be the same. Differences:\n%v", diff)
	}
}
