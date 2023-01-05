// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package cryptutil

import (
	"bytes"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/crypto/bcrypt"
)

// NewPasswordHasher returns a bcrypt hash generator.
// If size is greater than 0 the hashes are cached using a cache of the provided size.
func NewPasswordHasher(size int) (PasswordHasher, error) {
	if size > 0 {
		lruCache, err := lru.New[string, []byte](size)
		if err != nil {
			return nil, err
		}
		return &lruHashCache{
			generateFromPassword:   bcrypt.GenerateFromPassword,
			compareHashAndPassword: bcrypt.CompareHashAndPassword,
			hashCache:              lruCache,
		}, nil
	}
	return &passwordHashProvider{}, nil
}

type PasswordHasher interface {
	ReuseOrGenerateHash(password, existingHash []byte) ([]byte, error)
}

type generateFromPassword func(password []byte, cost int) ([]byte, error)
type compareHashAndPassword func(hashedPassword, password []byte) error
type lruHashCache struct {
	hashCache *lru.Cache[string, []byte]

	// only to be changed for unit tests
	generateFromPassword
	compareHashAndPassword
}

func (h *lruHashCache) ReuseOrGenerateHash(password, existingHash []byte) ([]byte, error) {
	key := string(password)

	if len(existingHash) > 0 {
		// Check if we have the hash in cache
		cachedHash := h.get(key)
		if cachedHash != nil && bytes.Equal(cachedHash, existingHash) {
			return existingHash, nil
		}

		// Check if the existing hash is valid
		if h.compareHashAndPassword(existingHash, password) == nil {
			// existing hash is valid, save it and return
			h.hashCache.Add(key, existingHash)
			return existingHash, nil
		}
	}

	// No existing hash or existing hash is not valid
	hash, err := h.generateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	h.hashCache.Add(key, hash)
	return hash, nil
}

func (h *lruHashCache) get(key string) []byte {
	cachedHash, exists := h.hashCache.Get(key)
	if !exists {
		return nil
	}
	return cachedHash
}

type passwordHashProvider struct{}

func (n *passwordHashProvider) ReuseOrGenerateHash(password, existingHash []byte) ([]byte, error) {
	if len(existingHash) > 0 && bcrypt.CompareHashAndPassword(existingHash, password) == nil {
		return existingHash, nil
	}
	return bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
}
