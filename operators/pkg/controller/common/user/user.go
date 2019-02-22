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
	Id() string
	PasswordMatches(hash []byte) bool
	PasswordHash() ([]byte, error)
	Roles() []string
}
