package comparison

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Equal compares two objects ignoring the TypeMeta and ResourceVersion. Often used for tests ensuring that we receive structs that match what we expect without
// runtime-specific information
// Does it make sense to consume metav1.Object or runtime.Object?
func Equal(a, b runtime.Object) bool {
	typemeta := cmpopts.IgnoreTypes(metav1.TypeMeta{})
	rv := cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")
	return cmp.Equal(a, b, typemeta, rv)
}

// Diff compares two objects ignoring the TypeMeta and ResourceVersion. Often used for tests ensuring that we receive structs that match what we expect without
// runtime-specific information
// Does it make sense to consume metav1.Object or runtime.Object?
func Diff(a, b runtime.Object) string {
	typemeta := cmpopts.IgnoreTypes(metav1.TypeMeta{})
	rv := cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")
	return cmp.Diff(a, b, typemeta, rv)
}

func AssertEqual(t *testing.T, a, b runtime.Object) {
	t.Helper()
	diff := Diff(a, b)
	if diff != "" {
		t.Errorf("Expected objects to be the same. Differences:\n%v", diff)
	}
}

func RequireEqual(t *testing.T, a, b runtime.Object) {
	t.Helper()
	diff := Diff(a, b)
	if diff != "" {
		t.Fatalf("Expected objects to be the same. Differences:\n%v", diff)
	}
}
