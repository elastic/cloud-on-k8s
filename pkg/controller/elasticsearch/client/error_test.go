// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_IsAPIError(t *testing.T) {
	assert.True(t, IsAPIError(&APIError{StatusCode: 404}))
	assert.True(t, IsAPIError(&APIError{}))
	assert.False(t, IsAPIError(errors.New("a simple error")))
	assert.False(t, IsAPIError(nil))
}