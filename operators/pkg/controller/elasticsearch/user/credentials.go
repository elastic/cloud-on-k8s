// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	common "github.com/elastic/k8s-operators/operators/pkg/controller/common/user"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/ghodss/yaml"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ElasticUsersFile is the name of the users file in the ES config dir.
	ElasticUsersFile = "users"
	// ElasticUsersRolesFile is the name of the users_roles file in the ES config dir.
	ElasticUsersRolesFile = "users_roles"
	// ElasticRolesFile is the name of the roles file in the ES config dir.
	ElasticRolesFile = "roles.yml"
)

// ElasticUsersRolesSecretName is the name of the secret containing all users and roles information in ES format.
func ElasticUsersRolesSecretName(ownerName string) string {
	return stringsutil.Concat(ownerName, "-es-roles-users")
}

// ElasticInternalUsersSecretName is the name of the secret containing the internal users' credentials.
func ElasticInternalUsersSecretName(ownerName string) string {
	return stringsutil.Concat(ownerName, "-internal-users")
}

// ElasticExternalUsersSecretName is the name of the secret containing the external users' credentials.
func ElasticExternalUsersSecretName(ownerName string) string {
	return stringsutil.Concat(ownerName, "-elastic-user")
}

// UserCredentials captures Elasticsearch user credentials and their representation in a k8s secret.
type UserCredentials interface {
	Secret() corev1.Secret
	Reset(secret corev1.Secret)
	NeedsUpdate(other corev1.Secret) bool
}

// ClearTextCredentials store a secret with clear text passwords.
type ClearTextCredentials struct {
	users  []User
	secret corev1.Secret
}

func keysEqual(v1, v2 map[string][]byte) bool {
	if len(v1) != len(v2) {
		return false
	}

	for k := range v1 {
		if _, ok := v2[k]; !ok {
			return false
		}
	}
	return true
}

// Reset resets the source of truth for these credentials.
func (c *ClearTextCredentials) Reset(secret corev1.Secret) {
	c.secret = secret
	for i := 0; i < len(c.users); i++ {
		old := c.users[i]
		pw := secret.Data[old.Id()]
		c.users[i] = New(old.Id(), Password(string(pw)), Roles(old.Roles()...))
	}
}

// NeedsUpdate is true for clear text credentials if the secret contains the same keys as the reference secret.
func (c *ClearTextCredentials) NeedsUpdate(other corev1.Secret) bool {
	// for generated secrets as long as the key exists we can work with it. Rotate secrets by deleting them (?)
	for _, user := range c.users {
		if _, ok := other.Data[user.Id()]; !ok {
			return true
		}
	}
	return false
}

// Users returns the users slice stored in the struct.
func (c *ClearTextCredentials) Users() []User {
	return c.users
}

// Secret returns the underlying secret.
func (c *ClearTextCredentials) Secret() corev1.Secret {
	return c.secret
}

// HashedCredentials store Elasticsearch user names and password hashes.
type HashedCredentials struct {
	users  []common.User
	secret corev1.Secret
}

// Reset resets the secrets of these credentials. Source of truth are the users though.
func (hc *HashedCredentials) Reset(secret corev1.Secret) {
	hc.secret = secret
}

// NeedsUpdate checks whether the secret data in other matches the user information in these credentials.
func (hc *HashedCredentials) NeedsUpdate(other corev1.Secret) bool {
	if !keysEqual(hc.secret.Data, other.Data) {
		return true
	}

	// Check for roles update
	otherRoles, found := other.Data[ElasticRolesFile]
	if !found {
		return true
	}
	if !bytes.Equal(otherRoles, hc.secret.Data[ElasticRolesFile]) {
		return true
	}

	// Check for users_roles update
	otherUsersRoles, found := other.Data[ElasticUsersRolesFile]
	if !found {
		return true
	}
	if !bytes.Equal(otherUsersRoles, hc.secret.Data[ElasticUsersRolesFile]) {
		return true
	}

	// Check for users update
	otherUsers := make(map[string][]byte)
	for _, user := range strings.Split(string(other.Data[ElasticUsersFile]), "\n") {
		userPw := strings.Split(user, ":")
		if len(userPw) != 2 { // corrupted data needs update, should always be pairs
			return true
		}
		otherUsers[userPw[0]] = []byte(userPw[1])
	}

	// Check for the addition or removal of users
	if len(hc.users) != len(otherUsers) {
		return true
	}

	// Check for user passwords update
	for _, u := range hc.users {
		otherPasswordBytes, ok := otherUsers[u.Id()]
		// this could turn out to be too expensive
		if !ok || !u.PasswordMatches(otherPasswordBytes) {
			return true
		}
	}

	return false
}

// Secret returns the underlying k8s secret.
func (hc *HashedCredentials) Secret() corev1.Secret {
	return hc.secret
}

// NewInternalUserCredentials creates a secret for the ES user used by the controller.
func NewInternalUserCredentials(es types.NamespacedName) *ClearTextCredentials {
	return usersToClearTextCredentials(es, ElasticInternalUsersSecretName(es.Name), internalUsers)
}

// NewExternalUserCredentials creates a secret for the Elastic user to be used by external users.
func NewExternalUserCredentials(es types.NamespacedName) *ClearTextCredentials {
	return usersToClearTextCredentials(es, ElasticExternalUsersSecretName(es.Name), externalUsers)
}

func usersToClearTextCredentials(es types.NamespacedName, secretName string, users []User) *ClearTextCredentials {
	data := make(map[string][]byte, len(users))
	for _, user := range users {
		data[user.Id()] = []byte(user.Password())
	}

	return &ClearTextCredentials{
		users: users,
		secret: corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: es.Namespace,
				Name:      secretName,
				Labels:    label.NewLabels(es),
			},
			Data: data,
		},
	}
}

// NewElasticUsersCredentialsAndRoles creates a k8s secret with user credentials and roles readable by ES
// for the given users.
func NewElasticUsersCredentialsAndRoles(
	es types.NamespacedName,
	users []common.User,
	roles map[string]client.Role,
) (*HashedCredentials, error) {

	// sort to avoid unnecessary diffs and API resource updates
	sort.SliceStable(users, func(i, j int) bool {
		return users[i].Id() < users[j].Id()
	})

	usersFileBytes, err := getUsersFileBytes(users)
	if err != nil {
		return &HashedCredentials{}, err
	}

	userRolesFileBytes, err := getUsersRolesFileBytes(users)
	if err != nil {
		return &HashedCredentials{}, err
	}

	rolesFileBytes, err := getRolesFileBytes(roles)
	if err != nil {
		return &HashedCredentials{}, err
	}

	return &HashedCredentials{
		users: users,
		secret: corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: es.Namespace,
				Name:      ElasticUsersRolesSecretName(es.Name),
				Labels:    label.NewLabels(es),
			},
			Data: map[string][]byte{
				ElasticUsersFile:      usersFileBytes,
				ElasticUsersRolesFile: userRolesFileBytes,
				ElasticRolesFile:      rolesFileBytes,
			},
		},
	}, nil
}

func getUsersFileBytes(users []common.User) ([]byte, error) {
	lines := make([]string, len(users))
	for i, user := range users {
		hash, err := user.PasswordHash()
		if err != nil {
			return nil, err
		}

		lines[i] = user.Id() + ":" + string(hash)
	}

	return []byte(strings.Join(lines, "\n")), nil
}

func getUsersRolesFileBytes(users []common.User) ([]byte, error) {
	rolesUsers := map[string][]string{}
	for _, user := range users {
		for _, role := range user.Roles() {
			if role == "" {
				return nil, fmt.Errorf("role not defined for user `%s`", user.Id())
			}

			roleUsers := rolesUsers[role]
			if roleUsers == nil {
				roleUsers = []string{}
			}
			rolesUsers[role] = append(roleUsers, user.Id())
		}
	}
	var lines []string
	for role, users := range rolesUsers {
		lines = append(lines, role+":"+strings.Join(users, ","))
	}

	// sort to avoid unnecessary diffs and API resource updates
	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i] < lines[j]
	})

	return []byte(strings.Join(lines, "\n")), nil
}

func getRolesFileBytes(roles map[string]client.Role) ([]byte, error) {
	rolesYamlBytes, err := yaml.Marshal(roles)
	if err != nil {
		return nil, err
	}

	return rolesYamlBytes, nil
}
