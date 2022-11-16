// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cryptutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

func Test_lruHashCache_ReuseOrGenerateHash(t *testing.T) {
	newHasher, err := NewPasswordHasher(2)
	assert.NoError(t, err)
	passwordHasher, ok := newHasher.(*lruHashCache)
	assert.True(t, ok)

	generatedHashes := 0
	passwordHasher.generateFromPassword = func(password []byte, cost int) ([]byte, error) {
		generatedHashes++
		return bcrypt.GenerateFromPassword(password, cost)
	}

	comparedPasswords := 0
	passwordHasher.compareHashAndPassword = func(hashedPassword, password []byte) error {
		comparedPasswords++
		return bcrypt.CompareHashAndPassword(hashedPassword, password)
	}

	hash1, err := passwordHasher.ReuseOrGenerateHash([]byte("password1"), nil)
	assert.NoError(t, err)
	assert.Nil(t, bcrypt.CompareHashAndPassword(hash1, []byte("password1")))
	// 1 password has just been generated
	assert.Equal(t, 1, generatedHashes)
	// No crypto comparison so far
	assert.Equal(t, 0, comparedPasswords)

	// stored hash is valid, should not be changed
	storedHash := "$2a$10$aAeQF8kG.AOrsp4mtzFQ4.1z5lvL0w8odQl2tvaxGdKkQTyHMOSEe"
	hash2, err := passwordHasher.ReuseOrGenerateHash([]byte("password2"), []byte(storedHash))
	assert.NoError(t, err)
	assert.Equal(t, storedHash, string(hash2))
	assert.Nil(t, bcrypt.CompareHashAndPassword(hash2, []byte("password2")))
	// Password has just been compared using the crypto function
	assert.Equal(t, 1, comparedPasswords)
	// No password generated
	assert.Equal(t, 1, generatedHashes)
	// 2 passwords in cache
	assert.Equal(t, 2, passwordHasher.hashCache.Len())

	// hash1 should still be in cache
	hash1_2, err := passwordHasher.ReuseOrGenerateHash([]byte("password1"), hash1)
	assert.NoError(t, err)
	assert.Nil(t, bcrypt.CompareHashAndPassword(hash1_2, []byte("password1")))
	// Hash should have been read from cache
	assert.Equal(t, 1, comparedPasswords)
	// No password generated
	assert.Equal(t, 1, generatedHashes)
	// 2 passwords in cache
	assert.Equal(t, 2, passwordHasher.hashCache.Len())

	hash3, err := passwordHasher.ReuseOrGenerateHash([]byte("password3"), nil)
	assert.NoError(t, err)
	assert.Nil(t, bcrypt.CompareHashAndPassword(hash3, []byte("password3")))
	// New password has been generated
	assert.Equal(t, 2, generatedHashes)
	// Cache size should still be 2
	assert.Equal(t, 2, passwordHasher.hashCache.Len())
}

func Test_passwordHashProvider_ReuseOrGenerateHash(t *testing.T) {
	passwordHasher, err := NewPasswordHasher(0)
	assert.NoError(t, err)
	_, ok := passwordHasher.(*passwordHashProvider)
	assert.True(t, ok)

	hash1, err := passwordHasher.ReuseOrGenerateHash([]byte("password1"), nil)
	assert.NoError(t, err)
	assert.Nil(t, bcrypt.CompareHashAndPassword(hash1, []byte("password1")))

	// stored hash is valid, should not be changed
	storedHash := "$2a$10$aAeQF8kG.AOrsp4mtzFQ4.1z5lvL0w8odQl2tvaxGdKkQTyHMOSEe"
	hash2, err := passwordHasher.ReuseOrGenerateHash([]byte("password2"), []byte(storedHash))
	assert.NoError(t, err)
	assert.Equal(t, storedHash, string(hash2))
}
