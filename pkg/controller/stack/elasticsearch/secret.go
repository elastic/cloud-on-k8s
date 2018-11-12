package elasticsearch

import (
	"sort"
	"strings"

	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/initcontainer"

	"k8s.io/apimachinery/pkg/util/rand"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ElasticUsersFile is the name of the users file in the ES config dir.
	ElasticUsersFile = "users"
	// ElasticUsersRolesFile is the name of the users_roles file in the ES config dir.
	ElasticUsersRolesFile = "users_roles"
	// ExternalUserName also known as the 'elastic' user.
	ExternalUserName = "elastic"
	// InternalControllerUserName a user to be used from this controller when interacting with ES.
	InternalControllerUserName = "elastic-internal"
	// InternalKibanaServerUserName is a user to be used by the Kibana server when interacting with ES.
	InternalKibanaServerUserName = "elastic-internal-kibana"
)

var (
	// LinkedFiles describe how the user related secrets are mapped into the pod's filesystem.
	LinkedFiles = initcontainer.LinkedFilesArray{
		Array: []initcontainer.LinkedFile{
			initcontainer.LinkedFile{
				Source: common.Concat(defaultSecretMountPath, "/", ElasticUsersFile),
				Target: common.Concat("/usr/share/elasticsearch/config", "/", ElasticUsersFile),
			},
			initcontainer.LinkedFile{
				Source: common.Concat(defaultSecretMountPath, "/", ElasticUsersRolesFile),
				Target: common.Concat("/usr/share/elasticsearch/config", "/", ElasticUsersRolesFile),
			},
		},
	}
)

// ElasticUsersSecretName is the name of the secret containing all users credentials in ES format.
func ElasticUsersSecretName(ownerName string) string {
	return common.Concat(ownerName, "-users")
}

// ElasticInternalUsersSecretName is the name of the secret containing the internal users' credentials
func ElasticInternalUsersSecretName(ownerName string) string {
	return common.Concat(ownerName, "-internal-users")
}

// UserCredentials captures Elasticsearch user credentials and their representation in a k8s secret.
type UserCredentials interface {
	Users() []client.User
	Secret() corev1.Secret
	Reset(secret corev1.Secret)
	NeedsUpdate(other corev1.Secret) bool
}

// ClearTextCredentials store a secret with clear text passwords.
type ClearTextCredentials struct {
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
}

// NeedsUpdate is true for clear text credentials if the secret contains the same keys as the reference secret.
func (c *ClearTextCredentials) NeedsUpdate(other corev1.Secret) bool {
	// for generated secrets as long as the key exists we can work with it. Rotate secrets by deleting them (?)
	return !keysEqual(c.secret.Data, other.Data)
}

// Users returns a slice of users based on secret as source of truth
func (c *ClearTextCredentials) Users() []client.User {
	var result []client.User
	for user, pw := range c.secret.Data {
		result = append(result, client.User{Name: user, Password: string(pw)})
	}
	return result
}

// Secret returns the underlying secret.
func (c *ClearTextCredentials) Secret() corev1.Secret {
	return c.secret
}

// HashedCredentials store Elasticsearch user names and password hashes.
type HashedCredentials struct {
	users  []client.User
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

	otherRoles, found := other.Data[ElasticUsersRolesFile]
	if !found {
		return true
	}
	if string(otherRoles) != string(hc.secret.Data[ElasticUsersRolesFile]) {
		return true
	}
	otherUsers := make(map[string][]byte)
	for _, user := range strings.Split(string(other.Data[ElasticUsersFile]), "\n") {
		userPw := strings.Split(user, ":")
		if len(userPw) != 2 { //corrupted data needs update, should always be pairs
			return true
		}
		otherUsers[userPw[0]] = []byte(userPw[1])
	}

	for _, u := range hc.users {
		bytes, ok := otherUsers[u.Name]
		// this could turn out to be too expensive
		if !ok || bcrypt.CompareHashAndPassword(bytes, []byte(u.Password)) != nil {
			return true
		}
	}

	return false
}

// Secret returns the underlying k8s secret.
func (hc *HashedCredentials) Secret() corev1.Secret {
	return hc.secret
}

// Users returns the user array stored in the struct
func (hc *HashedCredentials) Users() []client.User {
	return hc.users
}

// NewInternalUserCredentials creates a secret for the ES user used by the controller.
func NewInternalUserCredentials(s deploymentsv1alpha1.Stack) *ClearTextCredentials {

	return &ClearTextCredentials{
		secret: corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: s.Namespace,
				Name:      ElasticInternalUsersSecretName(s.Name),
				Labels:    NewLabels(s, false),
			},
			Data: map[string][]byte{
				InternalControllerUserName:   []byte(rand.String(24)),
				InternalKibanaServerUserName: []byte(rand.String(24)),
			},
		}}
}

// NewExternalUserCredentials creates a secret for the Elastic user to be used by external users.
func NewExternalUserCredentials(s deploymentsv1alpha1.Stack) *ClearTextCredentials {
	return &ClearTextCredentials{
		secret: corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: s.Namespace,
				Name:      common.Concat(s.Name, "-elastic-user"),
				Labels:    NewLabels(s, false),
			},
			Data: map[string][]byte{
				ExternalUserName: []byte(rand.String(24)),
			},
		},
	}

}

// NewElasticUsersCredentials creates a k8s secret with user credentials and roles readable by ES
// for the given users.
func NewElasticUsersCredentials(s deploymentsv1alpha1.Stack, users []client.User) (*HashedCredentials, error) {
	//sort to avoid unnecessary diffs and API resource updates
	sort.SliceStable(users, func(i, j int) bool {
		return users[i].Name < users[j].Name
	})
	hashedCreds, roles := strings.Builder{}, strings.Builder{}
	roles.WriteString("superuser:") //TODO all superusers -> role mappings
	for i, user := range users {
		hash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			return &HashedCredentials{}, err
		}

		notLast := i+1 < len(users)

		hashedCreds.WriteString(user.Name)
		hashedCreds.WriteString(":")
		hashedCreds.Write(hash)
		if notLast {
			hashedCreds.WriteString("\n")
		}

		roles.WriteString(user.Name)
		if notLast {
			roles.WriteString(",")
		}
	}

	return &HashedCredentials{
		users: users,
		secret: corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: s.Namespace,
				Name:      ElasticUsersSecretName(s.Name),
				Labels:    NewLabels(s, false),
			},
			Data: map[string][]byte{
				ElasticUsersFile:      []byte(hashedCreds.String()),
				ElasticUsersRolesFile: []byte(roles.String()),
			},
		},
	}, nil
}
