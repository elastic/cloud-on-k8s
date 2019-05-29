// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"bytes"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
)

const (
	// UserName is the field in the secret that contains the username.
	UserName = "name"
	// PasswordHash is the field in the secret that contains the hash of the password.
	PasswordHash = "passwordHash"
	// UserRoles contains the roles for the user as a comma separated list of strings.
	UserRoles = "userRoles"

	fieldNotFound = "field %s not found in secret %s/%s"
)

// ProvidedUser represents a user that is not created or managed by Elasticsearch.
// For example in the case of Kibana a user with the right role is created by the Kibana association controller.
type ProvidedUser struct {
	name     string
	password []byte
	roles    []string
}

// NewProvidedUserFromSecret reads a provided user from a secret.
func NewProvidedUserFromSecret(secret v1.Secret) (ProvidedUser, error) {
	user := ProvidedUser{}
	if len(secret.Data) == 0 {
		return user, fmt.Errorf("user secret %s/%s is empty", secret.Namespace, secret.Name)
	}

	if username, ok := secret.Data[UserName]; ok && len(username) > 0 {
		user.name = string(username)
	} else {
		return user, fmt.Errorf(fieldNotFound, UserName, secret.Namespace, secret.Name)
	}

	if password, ok := secret.Data[PasswordHash]; ok && len(password) > 0 {
		user.password = password
	} else {
		return user, fmt.Errorf(fieldNotFound, PasswordHash, secret.Namespace, secret.Name)
	}

	if roles, ok := secret.Data[UserRoles]; ok && len(roles) > 0 {
		user.roles = strings.Split(string(roles), ",")
	}

	return user, nil
}

// Id is the user id.
func (u ProvidedUser) Id() string {
	return u.name
}

// PasswordHash is the password hash and returns it or error.
func (u ProvidedUser) PasswordHash() ([]byte, error) {
	return u.password, nil
}

// PasswordMatches compares the given hash with the password of this user.
func (u *ProvidedUser) PasswordMatches(hash []byte) bool {
	return bytes.Equal([]byte(u.password), hash)
}

// Roles are any Elasticsearch roles associated with this user
func (u *ProvidedUser) Roles() []string {
	return u.roles
}
