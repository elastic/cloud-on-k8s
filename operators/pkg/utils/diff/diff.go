// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// package diff contains utils to diff two objects. it's pretty much a vendored version of testify's diff with an
// utility function to build the diff.
package diff

import (
	"fmt"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/errors"
)

type testifyAdapter struct {
	errs []error
}

func (t *testifyAdapter) Errorf(format string, args ...interface{}) {
	t.errs = append(t.errs, fmt.Errorf(format, args...))
}

func (t *testifyAdapter) ErrOrNil() error {
	return errors.NewAggregate(t.errs)
}

// NewDiff returns the difference between two objects, suitable for debugging.
func NewDiffAsError(expected, actual interface{}) error {
	adapter := &testifyAdapter{}
	assert.Equal(adapter, expected, actual)
	return adapter.ErrOrNil()
}
