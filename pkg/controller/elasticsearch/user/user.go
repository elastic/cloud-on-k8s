// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user/filerealm"
)

// user is a convenience struct to represent a file realm user.
type user struct {
	Name         string
	Password     []byte
	PasswordHash []byte
	Roles        []string
}

// Realm builds a file realm representation of this user.
func (u user) fileRealm() filerealm.Realm {
	realm := filerealm.New().WithUser(u.Name, u.PasswordHash)
	for _, role := range u.Roles {
		realm = realm.WithRole(role, []string{u.Name})
	}
	return realm
}

// users is just a list of users to which we attach convenience functions.
type users []user

// Realm builds a filerealm representation of the users.
func (users users) fileRealm() filerealm.Realm {
	fileRealm := filerealm.New()
	for _, u := range users {
		fileRealm = fileRealm.MergeWith(u.fileRealm())
	}
	return fileRealm
}

// userAuth returns an Elasticsearch userAuth struct for the given user.
func (users users) userAuth(userName string) (client.UserAuth, error) {
	for _, u := range users {
		if u.Name == userName {
			return client.UserAuth{Name: userName, Password: string(u.Password)}, nil
		}
	}
	return client.UserAuth{}, fmt.Errorf("user %s not found", userName)
}

// fromAssociatedUsers returns a list of user from the given associated users.
func fromAssociatedUsers(associatedUsers []AssociatedUser) users {
	users := make(users, 0, len(associatedUsers))
	for _, u := range associatedUsers {
		users = append(users, user{
			Name:         u.Name,
			PasswordHash: u.PasswordHash,
			Roles:        u.Roles,
		})
	}
	return users
}
