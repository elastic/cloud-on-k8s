// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewExternalUsers(t *testing.T) {
	users1 := newExternalUsers()
	users2 := newExternalUsers()

	// User names must be equals
	assert.Equal(t, users1[0].name, users2[0].name)
	// User passwords must be different
	assert.NotEqual(t, users1[0].password, users2[0].password)
}

func TestNewInternalUsers(t *testing.T) {
	users1 := newInternalUsers()
	users2 := newInternalUsers()

	// User names must be equals
	assert.Equal(t, users1[0].name, users2[0].name)
	// User passwords must be different
	assert.NotEqual(t, users1[0].password, users2[0].password)
}
