// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filerealm

import (
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	// UsersFile is the name of the users file in the ES config dir.
	UsersFile = "users"
	// UsersRolesFile is the name of the users_roles file in the ES config dir.
	UsersRolesFile = "users_roles"
)

// Realm is the file realm representation, containing user password hashes and role mapping.
type Realm struct {
	users      usersPasswordHashes
	usersRoles usersRoles
}

// New empty file realm.
func New() Realm {
	return Realm{
		users:      make(usersPasswordHashes),
		usersRoles: make(usersRoles),
	}
}

// MergedFrom builds an aggregated file realm from the given ones.
func MergedFrom(others ...Realm) Realm {
	return New().MergeWith(others...)
}

// FromSecret builds a file realm from the given secret.
func FromSecret(secret corev1.Secret) (Realm, error) {
	users, err := parseUsersPasswordHashes(k8s.GetSecretEntry(secret, UsersFile))
	if err != nil {
		return Realm{}, errors.Wrap(err, fmt.Sprintf("fail to parse users from secret %s", secret.Name))
	}
	usersRoles, err := parseUsersRoles(k8s.GetSecretEntry(secret, UsersRolesFile))
	if err != nil {
		return Realm{}, errors.Wrap(err, fmt.Sprintf("fail to parse users roles from secret %s", secret.Name))
	}
	return Realm{users: users, usersRoles: usersRoles}, nil
}

// MergeWith merges multiple file realms together, giving priority to the last provided one.
func (f Realm) MergeWith(others ...Realm) Realm {
	for _, other := range others {
		f.users = f.users.mergeWith(other.users)
		f.usersRoles = f.usersRoles.mergeWith(other.usersRoles)
	}
	return f
}

// WithUser upserts the given user to the file realm.
func (f Realm) WithUser(name string, passwordHash []byte) Realm {
	f.users = f.users.mergeWith(usersPasswordHashes{name: passwordHash})
	return f
}

// WithRole adds the given role to the file realm, merging with existing users.
func (f Realm) WithRole(name string, users []string) Realm {
	f.usersRoles = f.usersRoles.mergeWith(usersRoles{name: users})
	return f
}

// PasswordHashForUser returns the password hash for the given user, or nil if the user doesn't exist.
func (f Realm) PasswordHashForUser(userName string) []byte {
	return f.users[userName]
}

// FileBytes returns a map with the content of the 2 file realm files.
func (f Realm) FileBytes() map[string][]byte {
	return map[string][]byte{
		UsersFile:      f.users.fileBytes(),
		UsersRolesFile: f.usersRoles.fileBytes(),
	}
}

// UserNames returns the list of user names in this file realm.
func (f Realm) UserNames() []string {
	names := make([]string, 0, len(f.users))
	for name := range f.users {
		names = append(names, name)
	}
	return names
}
