package elasticsearch

import (
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

// NewInternalUserSecret creates a secret for the ES user used by the controller
func NewInternalUserSecret(s deploymentsv1alpha1.Stack) corev1.Secret {
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Namespace,
			Name:      ElasticInternalUsersSecretName(s.Name),
			Labels:    NewLabels(s, false),
		},
		Data: map[string][]byte{
			InternalControllerUserName:   []byte(rand.String(24)),
			InternalKibanaServerUserName: []byte(rand.String(24)),
		},
	}
}

// NewExternalUserSecret creates a secret for the Elastic user to be used by external users.
func NewExternalUserSecret(s deploymentsv1alpha1.Stack) corev1.Secret {
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Namespace,
			Name:      common.Concat(s.Name, "-elastic-user"),
			Labels:    NewLabels(s, false),
		},
		Data: map[string][]byte{
			ExternalUserName: []byte(rand.String(24)),
		},
	}
}

// NewUsersFromSecret maps a given secret into an array of  users.
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
	roles.WriteString("superuser:") //TODO all superusers -> role mappings
	for i, user := range users {
		hash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			return corev1.Secret{}, err
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

	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Namespace,
			Name:      ElasticUsersSecretName(s.Name),
			Labels:    NewLabels(s, false),
		},
		Data: map[string][]byte{
			ElasticUsersFile:      []byte(hashedCreds.String()),
			ElasticUsersRolesFile: []byte(roles.String()),
		},
	}, nil
}
