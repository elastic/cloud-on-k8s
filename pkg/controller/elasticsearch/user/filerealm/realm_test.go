// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filerealm

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func Test_FileRealm_MergeWith(t *testing.T) {
	merged := Realm{
		users:      usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("hash2")},
		usersRoles: roleUsersMapping{"role1": []string{"user1"}, "role2": []string{"user2"}},
	}.MergeWith(
		Realm{
			users:      usersPasswordHashes{"user3": []byte("hash3"), "user2": []byte("anotherhash2")},
			usersRoles: roleUsersMapping{"role1": []string{"user1", "user3"}, "role2": []string{"user2"}},
		},
		Realm{
			users:      usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("yetanotherhash2")},
			usersRoles: roleUsersMapping{"role3": []string{"user1", "user2", "user3"}},
		},
	)
	require.Equal(t, Realm{
		// contains all users, last one has priority
		users: usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("yetanotherhash2"), "user3": []byte("hash3")},
		// contains all roles, with user list merged and sorted
		usersRoles: roleUsersMapping{"role1": []string{"user1", "user3"}, "role2": []string{"user2"}, "role3": []string{"user1", "user2", "user3"}},
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

	c := k8s.WrappedFakeClient(&secret)
	actualRealm, err := FromSecret(c, types.NamespacedName{Namespace: "ns", Name: "fileRealmSecret"})
	require.NoError(t, err)

	expectedRealm := Realm{
		users:      usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("hash2")},
		usersRoles: roleUsersMapping{"role1": []string{"user1", "user2"}, "role2": []string{"user1"}, "role3": nil},
	}
	require.Equal(t, expectedRealm, actualRealm)
}
