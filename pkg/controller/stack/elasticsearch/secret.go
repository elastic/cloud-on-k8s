package elasticsearch

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/rand"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ElasticUsers      = "users"
	ElasticUsersRoles = "users_roles"
	InternalUserName  = "elastic-internal"
)

// NewInternalUserSecret creates a secret for the ES user used by the controller
func NewInternalUserSecret(s deploymentsv1alpha1.Stack) corev1.Secret {
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Namespace,
			Name:      common.Concat(s.Name, "-internal-user-secret"),
			Labels:    NewLabels(s, false),
		},
		// TODO should this hold multiple internal users?
		StringData: map[string]string{
			InternalUserName: rand.String(24),
		},
	}
}

// NewUsersFromSecret maps a given secret into a User struct.
func NewUsersFromSecret(secret corev1.Secret) []client.User {

	var result []client.User
	for user, pw := range secret.Data {
		result = append(result, client.User{Name: user, Password: string(pw)})
	}
	return result
}

// NewElasticUsersSecret creates a k8s secret with user credentials and roles readable by ES
// for the given users.
func NewElasticUsersSecret(s deploymentsv1alpha1.Stack, users []client.User) (corev1.Secret, error) {
	hashedCreds, roles := strings.Builder{}, strings.Builder{}
	prefix, _ := roles.WriteString("superuser:") //TODO all superusers -> role mappings
	for _, user := range users {
		hash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			return corev1.Secret{}, err
		}
		hashedCreds.WriteString(user.Name)
		hashedCreds.WriteString(":")
		hashedCreds.Write(hash)
		hashedCreds.WriteString("\n")

		rolesIndex := roles.Len()
		if rolesIndex > prefix {
			roles.WriteString(",")
		}
		roles.WriteString(user.Name)
	}

	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Namespace,
			Name:      common.Concat(s.Name, "-users"),
			Labels:    NewLabels(s, false),
		},
		Data: map[string][]byte{
			ElasticUsers:      []byte(hashedCreds.String()),
			ElasticUsersRoles: []byte(roles.String()),
		},
	}, nil
}
