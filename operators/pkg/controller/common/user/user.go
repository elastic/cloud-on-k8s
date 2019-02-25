// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import "k8s.io/apimachinery/pkg/util/rand"

// RandomPasswordBytes generates a random password
func RandomPasswordBytes() []byte {
	return []byte(rand.String(24))
}

// User common interface representing Elasticsearch users of different origin (internal/external)
type User interface {
	// Id is the user id (to avoid name clashes with Name attribute of k8s resources)
	Id() string
	// PasswordMatches compares the given hash to the password of this user. Exists to abstract over user representations with clear text
	// passwords and those jus with a hash in which case both hashes need to match byte for byte.
	PasswordMatches(hash []byte) bool
	// PasswordHash computes a password hash and returns it or error.
	PasswordHash() ([]byte, error)
	// Roles are any Elasticsearch roles associated with this user
	Roles() []string
}
