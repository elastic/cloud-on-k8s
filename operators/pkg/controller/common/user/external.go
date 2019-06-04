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

// ExternalUser represents a user that is not created or managed by Elasticsearch.
// For example in the case of Kibana a user with the right role is provided by the Kibana association controller.
type ExternalUser struct {
	name     string
	password []byte
	roles    []string
}

// NewExternalUserFromSecret reads an external user from a secret.
func NewExternalUserFromSecret(secret v1.Secret) (ExternalUser, error) {
	user := ExternalUser{}
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
func (u ExternalUser) Id() string {
	return u.name
}

// PasswordHash is the password hash and returns it or error.
func (u ExternalUser) PasswordHash() ([]byte, error) {
	return u.password, nil
}

// PasswordMatches compares the given hash with the password of this user.
func (u *ExternalUser) PasswordMatches(hash []byte) bool {
	return bytes.Equal([]byte(u.password), hash)
}

// Roles are any Elasticsearch roles associated with this user
func (u *ExternalUser) Roles() []string {
	return u.roles
}
