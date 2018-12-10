package helpers

import (
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/utils/retry"
	"github.com/stretchr/testify/assert"
)

const (
	defaultRetryDelay = 3 * time.Second
	defaultTimeout    = 3 * time.Minute
)

// ExitOnErr exits with code 1 if the given error is not nil
func ExitOnErr(err error) {
	if err != nil {
		fmt.Println(err)
		fmt.Println("Exiting.")
		os.Exit(1)
	}
}

// Eventually runs the given function until success,
// with a default timeout
func Eventually(f func() error) func(*testing.T) {
	return func(t *testing.T) {
		fmt.Printf("Retries (%s timeout): ", defaultTimeout)
		err := retry.UntilSuccess(func() error {
			fmt.Print(".") // super modern progress bar 2.0!
			return f()
		}, defaultTimeout, defaultRetryDelay)
		fmt.Println()
		assert.NoError(t, err)
	}
}

// isEmpty gets whether the specified object is considered empty or not.
// Comes from https://github.com/stretchr/testify/blob/master/assert/assertions.go
func isEmpty(object interface{}) bool {
	// get nil case out of the way
	if object == nil {
		return true
	}

	objValue := reflect.ValueOf(object)

	switch objValue.Kind() {
	// collection types are empty when they have no element
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice:
		return objValue.Len() == 0
	// pointers are empty if nil or if the value they point to is empty
	case reflect.Ptr:
		if objValue.IsNil() {
			return true
		}
		deref := objValue.Elem().Interface()
		return isEmpty(deref)
	// for all other types, compare against the zero value
	default:
		zero := reflect.Zero(objValue.Type())
		return reflect.DeepEqual(object, zero.Interface())
	}
}

// SameUnorderedElements checks that both given slices contain the same elements
// Based on https://github.com/stretchr/testify/blob/master/assert/assertions.go
func CheckSameUnorderedElements(listA interface{}, listB interface{}) error {
	if isEmpty(listA) && isEmpty(listB) {
		return nil
	}

	aKind := reflect.TypeOf(listA).Kind()
	bKind := reflect.TypeOf(listB).Kind()

	if aKind != reflect.Array && aKind != reflect.Slice {
		return fmt.Errorf("%q has an unsupported type %s", listA, aKind)
	}

	if bKind != reflect.Array && bKind != reflect.Slice {
		return fmt.Errorf("%q has an unsupported type %s", listB, bKind)
	}

	aValue := reflect.ValueOf(listA)
	bValue := reflect.ValueOf(listB)

	aLen := aValue.Len()
	bLen := bValue.Len()

	if aLen != bLen {
		return fmt.Errorf("lengths don't match: %d != %d", aLen, bLen)
	}

	// Mark indexes in bValue that we already used
	visited := make([]bool, bLen)
	for i := 0; i < aLen; i++ {
		element := aValue.Index(i).Interface()
		found := false
		for j := 0; j < bLen; j++ {
			if visited[j] {
				continue
			}
			if assert.ObjectsAreEqual(bValue.Index(j).Interface(), element) {
				visited[j] = true
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("element %s appears more times in %s than in %s", element, aValue, bValue)
		}
	}

	return nil
}
