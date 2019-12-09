package comparison

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Equal compares two objects ignoring the TypeMeta and ResourceVersion. Often used for tests ensuring that we receive structs that match what we expect without
// runtime-specific information
// Does it make sense to consume metav1.Object or runtime.Object?
func Equal(a, b metav1.Object) bool {
	typemeta := cmpopts.IgnoreTypes(metav1.TypeMeta{})
	rv := cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")
	return cmp.Equal(a, b, typemeta, rv)
}

// Diff compares two objects ignoring the TypeMeta and ResourceVersion. Often used for tests ensuring that we receive structs that match what we expect without
// runtime-specific information
// Does it make sense to consume metav1.Object or runtime.Object?
func Diff(a, b metav1.Object) string {
	typemeta := cmpopts.IgnoreTypes(metav1.TypeMeta{})
	rv := cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")
	return cmp.Diff(a, b, typemeta, rv)
}
