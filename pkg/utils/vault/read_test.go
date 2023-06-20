// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package vault

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Get(t *testing.T) {
	c, _ := newMockClient(t, "secret", "42")()

	secret, err := Get(c, "fakepath", "secret")
	assert.NoError(t, err)
	assert.Equal(t, "42", secret)

	_, err = Get(c, "fakepath", "unknown")
	assert.Error(t, err)
}

func Test_GetMany(t *testing.T) {
	c, _ := newMockClient(t,
		"secret", "42",
		"token", "24",
	)()

	secrets, err := GetMany(c, "fakepath", "secret")
	assert.NoError(t, err)
	assert.Len(t, secrets, 1)
	assert.Equal(t, "42", secrets[0])

	secrets, err = GetMany(c, "fakepath", "secret", "token")
	assert.NoError(t, err)
	assert.Len(t, secrets, 2)
	assert.Equal(t, "42", secrets[0])
	assert.Equal(t, "24", secrets[1])

	_, err = GetMany(c, "fakepath", "unknown")
	assert.Error(t, err)

	_, err = GetMany(c, "fakepath", "key", "unknown")
	assert.Error(t, err)
}
