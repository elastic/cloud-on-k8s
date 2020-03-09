// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"testing"

	"github.com/stretchr/testify/require"

	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user/filerealm"
)

func Test_user_FileRealm(t *testing.T) {
	user := user{
		Name:         "user1",
		Password:     []byte("password1"),
		PasswordHash: []byte("password1Hash"),
		Roles:        []string{"role1", "role2"},
	}
	expected := filerealm.New().
		WithUser("user1", []byte("password1Hash")).
		WithRole("role1", []string{"user1"}).
		WithRole("role2", []string{"user1"})

	require.Equal(t, expected, user.fileRealm())
}

func Test_users_FileRealm(t *testing.T) {
	users := users{
		{
			Name:         "user1",
			Password:     []byte("password1"),
			PasswordHash: []byte("password1Hash"),
			Roles:        []string{"role1", "role2"},
		},
		{
			Name:         "user2",
			Password:     []byte("password2"),
			PasswordHash: []byte("password2Hash"),
			Roles:        []string{"role2", "role3"},
		},
	}
	expected := filerealm.New().
		WithUser("user1", []byte("password1Hash")).
		WithUser("user2", []byte("password2Hash")).
		WithRole("role1", []string{"user1"}).
		WithRole("role2", []string{"user1", "user2"}).
		WithRole("role3", []string{"user2"})

	require.Equal(t, expected, users.fileRealm())
}

func Test_users_UserAuth(t *testing.T) {
	users := users{
		{
			Name:         "user1",
			Password:     []byte("password1"),
			PasswordHash: []byte("password1Hash"),
			Roles:        []string{"role1", "role2"},
		},
		{
			Name:         "user2",
			Password:     []byte("password2"),
			PasswordHash: []byte("password2Hash"),
			Roles:        []string{"role2", "role3"},
		},
	}

	auth, err := users.userAuth("user1")
	require.NoError(t, err)
	require.Equal(t, esclient.UserAuth{
		Name:     "user1",
		Password: "password1",
	}, auth)

	// non-existing user should return an error
	_, err = users.userAuth("unknown-user")
	require.Error(t, err, "user unknown-user not found")
}

func Test_user_fileRealm(t *testing.T) {
	user := user{
		Name:         "username",
		Password:     []byte("password"),
		PasswordHash: []byte("passwordhash"),
		Roles:        []string{"role1", "role2"},
	}
	expected := filerealm.New().
		WithUser("username", []byte("passwordhash")).
		WithRole("role1", []string{"username"}).
		WithRole("role2", []string{"username"})
	require.Equal(t, expected, user.fileRealm())
}

func Test_users_fileRealm(t *testing.T) {
	users := users{
		{
			Name:         "username",
			Password:     []byte("password"),
			PasswordHash: []byte("passwordhash"),
			Roles:        []string{"role1", "role2"},
		},
		{
			Name:         "username2",
			Password:     []byte("password2"),
			PasswordHash: []byte("passwordhash2"),
			Roles:        []string{"role1", "role3"},
		},
	}
	expected := filerealm.New().
		WithUser("username", []byte("passwordhash")).
		WithUser("username2", []byte("passwordhash2")).
		WithRole("role1", []string{"username", "username2"}).
		WithRole("role2", []string{"username"}).
		WithRole("role3", []string{"username2"})
	require.Equal(t, expected, users.fileRealm())
}

func Test_users_userAuth(t *testing.T) {
	users := users{
		{
			Name:         "username",
			Password:     []byte("password"),
			PasswordHash: []byte("passwordhash"),
			Roles:        []string{"role1", "role2"},
		},
		{
			Name:         "username2",
			Password:     []byte("password2"),
			PasswordHash: []byte("passwordhash2"),
			Roles:        []string{"role1", "role3"},
		},
	}
	expected := esclient.UserAuth{
		Name:     "username2",
		Password: "password2",
	}
	actual, err := users.userAuth("username2")
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func Test_fromAssociatedUsers(t *testing.T) {
	associatedUsers := []AssociatedUser{
		{
			Name:         "user1",
			PasswordHash: []byte("hash1"),
			Roles:        []string{"role1", "role2"},
		},
		{
			Name:         "user2",
			PasswordHash: []byte("hash2"),
			Roles:        []string{"role1", "role2"},
		},
	}
	expected := users{
		{
			Name:         "user1",
			PasswordHash: []byte("hash1"),
			Roles:        []string{"role1", "role2"},
		},
		{
			Name:         "user2",
			PasswordHash: []byte("hash2"),
			Roles:        []string{"role1", "role2"},
		},
	}
	require.Equal(t, expected, fromAssociatedUsers(associatedUsers))
}
