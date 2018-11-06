package elasticsearch

import (
	"strings"

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

func NewInternalUserSecret(s deploymentsv1alpha1.Stack) corev1.Secret {
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Namespace,
			Name:      common.Concat(s.Name, "-internal-user-secret"),
			Labels:    NewLabels(s, false),
		},
		// TODO should this hold multiple internal users?
		StringData: map[string]string{
			InternalUserName: "TODO-random-string",
		},
	}
}

func NewUsersFromSecret(secret corev1.Secret) ([]client.User, error) {

	var result []client.User
	for user, pw := range secret.Data {
		// TODO should this be base64 encoded?
		//decoded, err := base64.StdEncoding.DecodeString(string(pw))
		//if err != nil {
		//	return result, err
		//}
		result = append(result, client.User{Name: user, Password: string(pw)})
	}
	return result, nil
}

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

	//TODO don't use string builder build into byte[] directly

	//userContent := base64.StdEncoding.EncodeToString([]byte(hashedCreds.String()))
	//roleContent := base64.StdEncoding.EncodeToString([]byte(roles.String()))

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
