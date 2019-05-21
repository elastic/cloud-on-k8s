package user

import (
	"bytes"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
)

const (
	UserName     = "name"
	PasswordHash = "passwordHash"
	UserRoles    = "userRoles"
)

type ProvidedUser struct {
	name     string
	password []byte
	roles    []string
}

// FromSecret creates a user from a secret.
func NewFromSecret(secret v1.Secret) (ProvidedUser, error) {
	user := ProvidedUser{}
	if len(secret.Data) == 0 {
		return user, fmt.Errorf("user secret %s/%s is empty", secret.Namespace, secret.Name)
	}

	if username, ok := secret.Data[UserName]; ok && len(username) > 0 {
		user.name = string(username)
	}

	if password, ok := secret.Data[PasswordHash]; ok && len(password) > 0 {
		user.password = password
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
	// this is tricky: we don't have password at hand so the hash has to match byte for byte. This might lead to false
	// negatives where the hash matches the password but a different salt or work factor was used.
	return bytes.Equal([]byte(u.password), hash)
}

// Roles are any Elasticsearch roles associated with this user
func (u *ProvidedUser) Roles() []string {
	return u.roles
}
