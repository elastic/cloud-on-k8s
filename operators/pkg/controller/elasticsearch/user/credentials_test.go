// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"sort"
	"strings"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/user"
	"k8s.io/apimachinery/pkg/types"

	"golang.org/x/crypto/bcrypt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/stretchr/testify/assert"
)

var (
	testES = types.NamespacedName{
		Name:      "my-cluster",
		Namespace: "default",
	}
	testUser = []user.User{New("foo", Password("bar"), Roles("role1"))}
	testRole = map[string]client.Role{
		"role1": {Cluster: []string{"all"}},
	}
)

func TestNewUserSecrets(t *testing.T) {
	elasticUsers, err := NewElasticUsersCredentialsAndRoles(testES, testUser, testRole)
	assert.NoError(t, err)

	tests := []struct {
		subject      UserCredentials
		expectedName string
		expectedKeys []string
	}{
		{
			subject:      NewInternalUserCredentials(testES),
			expectedName: "my-cluster-es-internal-users",
			expectedKeys: []string{InternalControllerUserName, InternalKeystoreUserName, InternalProbeUserName},
		},
		{
			subject:      NewExternalUserCredentials(testES),
			expectedName: "my-cluster-es-elastic-user",
			expectedKeys: []string{ExternalUserName},
		},
		{
			subject:      elasticUsers,
			expectedName: "my-cluster-es-xpack-file-realm",
			expectedKeys: []string{ElasticRolesFile, ElasticUsersFile, ElasticUsersRolesFile},
		},
	}

	for _, tt := range tests {
		secret := tt.subject.Secret()
		assert.Equal(t, tt.expectedName, secret.Name)
		var keys []string
		for k, v := range secret.Data {
			keys = append(keys, k)
			assert.NotEmpty(t, v)
		}
		sort.Strings(keys)
		assert.EqualValues(t, tt.expectedKeys, keys)
	}

}

func TestNewElasticUsersSecret(t *testing.T) {
	creds, err := NewElasticUsersCredentialsAndRoles(testES, testUser, testRole)
	assert.NoError(t, err)
	assert.Equal(t, "role1:foo", string(creds.Secret().Data[ElasticUsersRolesFile]))

	for _, user := range strings.Split(string(creds.Secret().Data[ElasticUsersFile]), "\n") {
		userPw := strings.Split(user, ":")
		assert.Equal(t, "foo", userPw[0])
		assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(userPw[1]), []byte("bar")))
	}
	assert.Equal(t, "role1:\n  cluster:\n  - all\n", string(creds.Secret().Data[ElasticRolesFile]))
}

func newTestCredentials(t *testing.T, users []user.User) UserCredentials {
	creds, err := NewElasticUsersCredentialsAndRoles(testES, users, testRole)
	assert.NoError(t, err)
	return creds
}

func TestNeedsUpdate(t *testing.T) {
	otherUser := New("baz", Password("yolo"))

	tests := []struct {
		desc        string
		subject1    UserCredentials
		subject2    UserCredentials
		needsUpdate bool
	}{
		{
			desc:        "internal clear text creds don't need update even if they contain different passwords (secret is source of truth)",
			subject1:    NewInternalUserCredentials(testES),
			subject2:    NewInternalUserCredentials(testES),
			needsUpdate: false,
		},
		{
			desc:        "external clear text creds don't need update even if they contain different passwords (secret is source of truth)",
			subject1:    NewExternalUserCredentials(testES),
			subject2:    NewExternalUserCredentials(testES),
			needsUpdate: false,
		},
		{
			desc:        "hashed creds: different hash but same password does not warrant an update of the secret",
			subject1:    newTestCredentials(t, testUser),
			subject2:    newTestCredentials(t, testUser),
			needsUpdate: false,
		},
		{
			desc:        "hashed creds: changed password warrants an update of the secret",
			subject1:    newTestCredentials(t, testUser),
			subject2:    newTestCredentials(t, []user.User{New("foo", Roles("role1"))}),
			needsUpdate: true,
		},
		{
			desc:        "hashed creds: changed role warrants an update of the secret",
			subject1:    newTestCredentials(t, testUser),
			subject2:    newTestCredentials(t, []user.User{New("foo", Roles("role2"))}),
			needsUpdate: true,
		},
		{
			desc:        "hashed creds: adding a user warrants an update of the secret",
			subject1:    newTestCredentials(t, testUser),
			subject2:    newTestCredentials(t, append(testUser, otherUser)),
			needsUpdate: true,
		},
		{
			desc:        "hashed creds: order of user credentials should not matter",
			subject1:    newTestCredentials(t, []user.User{testUser[0], otherUser}),
			subject2:    newTestCredentials(t, []user.User{otherUser, testUser[0]}),
			needsUpdate: false,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.needsUpdate, tt.subject1.NeedsUpdate(tt.subject2.Secret()), tt.desc)
	}
}
