// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"golang.org/x/crypto/bcrypt"
)

// User captures Elasticsearch user credentials.
type User struct {
	name     string
	password string
	roles    []string
}

// Attr an attribute to configure a user struct.
type Attr func(*User)

// Password sets the password of a new user to pw.
func Password(pw string) Attr {
	return func(u *User) {
		u.password = pw
	}
}

// Roles sets the roles of a new user to roles.
func Roles(roles ...string) Attr {
	return func(u *User) {
		u.roles = roles
	}
}

// New creates a new user with the given attributes.
func New(name string, setters ...Attr) User {
	result := User{
		name:     name,
		password: string(user.RandomPasswordBytes()),
	}
	for _, setter := range setters {
		setter(&result)
	}
	return result
}

var _ user.User = User{}

// Id is the user id.
func (u User) Id() string {
	return u.name
}

// PasswordHash computes a password hash and returns it or error.
func (u User) PasswordHash() ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(u.password), bcrypt.DefaultCost)
}

// PasswordMatches compares the given hash with the password of this user.
func (u User) PasswordMatches(hash []byte) bool {
	return bcrypt.CompareHashAndPassword(hash, []byte(u.password)) == nil
}

// Password returns the password of this user.
func (u User) Password() string {
	return string(u.password)
}

// Roles returns any roles of this user.
func (u User) Roles() []string {
	return u.roles
}

// Auth creates an auth object for the Elasticsearch client to use.
func (u User) Auth() client.UserAuth {
	return client.UserAuth{
		Name:     u.name,
		Password: u.password,
	}
}
