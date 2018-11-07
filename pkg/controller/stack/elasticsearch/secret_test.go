package elasticsearch

import (
	"sort"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	mockStack = deploymentsv1alpha1.Stack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-stack",
			Namespace: "default",
		}}
	testUser = []client.User{client.User{Name: "foo", Password: "bar"}}
)

func TestNewUserSecrets(t *testing.T) {

	elasticUsers, err := NewElasticUsersSecret(mockStack, testUser)
	assert.NoError(t, err)

	tests := []struct {
		subject      corev1.Secret
		expectedName string
		expectedKeys []string
	}{
		{
			subject:      NewInternalUserSecret(mockStack),
			expectedName: "my-stack-internal-users",
			expectedKeys: []string{InternalControllerUserName, InternalKibanaServerUserName},
		},
		{
			subject:      NewExternalUserSecret(mockStack),
			expectedName: "my-stack-elastic-user",
			expectedKeys: []string{ExternalUserName},
		},
		{
			subject:      elasticUsers,
			expectedName: "my-stack-users",
			expectedKeys: []string{ElasticUsersFile, ElasticUsersRolesFile},
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expectedName, tt.subject.Name)
		var keys []string
		for k := range tt.subject.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		assert.EqualValues(t, tt.expectedKeys, keys)
	}

}

func TestNewElasticUsersSecret(t *testing.T) {
	secret, err := NewElasticUsersSecret(mockStack, testUser)
	assert.NoError(t, err)
	assert.Equal(t, "superuser:foo", string(secret.Data[ElasticUsersRolesFile]))

	for _, user := range strings.Split(string(secret.Data[ElasticUsersFile]), "\n") {
		userPw := strings.Split(user, ":")
		assert.Equal(t, "foo", userPw[0])
		assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(userPw[1]), []byte("bar")))
	}

}
