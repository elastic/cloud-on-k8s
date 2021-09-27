// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filerealm

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_FileRealm_MergeWith(t *testing.T) {
	merged := Realm{
		users:      usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("hash2")},
		usersRoles: usersRoles{"role1": []string{"user1"}, "role2": []string{"user2"}},
	}.MergeWith(
		Realm{
			users:      usersPasswordHashes{"user3": []byte("hash3"), "user2": []byte("anotherhash2")},
			usersRoles: usersRoles{"role1": []string{"user1", "user3"}, "role2": []string{"user2"}},
		},
		Realm{
			users:      usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("yetanotherhash2")},
			usersRoles: usersRoles{"role3": []string{"user1", "user2", "user3"}},
		},
	)
	require.Equal(t, Realm{
		// contains all users, last one has priority
		users: usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("yetanotherhash2"), "user3": []byte("hash3")},
		// contains all roles, with user list merged and sorted
		usersRoles: usersRoles{"role1": []string{"user1", "user3"}, "role2": []string{"user2"}, "role3": []string{"user1", "user2", "user3"}},
	}, merged)
}

func Test_FileRealm_PasswordHashForUser(t *testing.T) {
	r := Realm{users: usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("hash2")}}
	require.Equal(t, []byte("hash1"), r.PasswordHashForUser("user1"))
	require.Equal(t, []byte("hash2"), r.PasswordHashForUser("user2"))
	require.Equal(t, []byte(nil), r.PasswordHashForUser("unknown-user"))
}

func Test_FromSecret(t *testing.T) {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "fileRealmSecret"},
		Data: map[string][]byte{
			UsersRolesFile: []byte(`role1:user1,user2
role2:user1
role3:`),
			UsersFile: []byte(`user1:hash1
user2:hash2
`),
		},
	}

	actualRealm, err := FromSecret(secret)
	require.NoError(t, err)

	expectedRealm := Realm{
		users:      usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("hash2")},
		usersRoles: usersRoles{"role1": []string{"user1", "user2"}, "role2": []string{"user1"}, "role3": nil},
	}
	require.Equal(t, expectedRealm, actualRealm)
}
