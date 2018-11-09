package elasticsearch

import (
	"sort"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"github.com/stretchr/testify/assert"
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

	elasticUsers, err := NewElasticUsersCredentials(mockStack, testUser)
	assert.NoError(t, err)

	tests := []struct {
		subject      UserCredentials
		expectedName string
		expectedKeys []string
	}{
		{
			subject:      NewInternalUserCredentials(mockStack),
			expectedName: "my-stack-internal-users",
			expectedKeys: []string{InternalControllerUserName, InternalKibanaServerUserName},
		},
		{
			subject:      NewExternalUserCredentials(mockStack),
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
		assert.Equal(t, tt.expectedName, tt.subject.Secret().Name)
		var keys []string
		for k := range tt.subject.Secret().Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		assert.EqualValues(t, tt.expectedKeys, keys)
	}

}

func TestNewElasticUsersSecret(t *testing.T) {
	creds, err := NewElasticUsersCredentials(mockStack, testUser)
	assert.NoError(t, err)
	assert.Equal(t, "superuser:foo", string(creds.Secret().Data[ElasticUsersRolesFile]))

	for _, user := range strings.Split(string(creds.Secret().Data[ElasticUsersFile]), "\n") {
		userPw := strings.Split(user, ":")
		assert.Equal(t, "foo", userPw[0])
		assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(userPw[1]), []byte("bar")))
	}

}

func TestNeedsUpdate(t *testing.T) {

	elasticUsers1, err := NewElasticUsersCredentials(mockStack, testUser)
	assert.NoError(t, err)
	elasticUsers2, err := NewElasticUsersCredentials(mockStack, testUser) //different hash but same password
	assert.NoError(t, err)
	elasticUsers3, err := NewElasticUsersCredentials(mockStack, []client.User{client.User{Name: "foo", Password: "changed!"}})
	assert.NoError(t, err)

	tests := []struct {
		subject1    UserCredentials
		subject2    UserCredentials
		needsUpdate bool
	}{
		{
			subject1:    NewInternalUserCredentials(mockStack),
			subject2:    NewInternalUserCredentials(mockStack),
			needsUpdate: false,
		},
		{
			subject1:    NewExternalUserCredentials(mockStack),
			subject2:    NewExternalUserCredentials(mockStack),
			needsUpdate: false,
		},
		{
			subject1:    elasticUsers1,
			subject2:    elasticUsers2,
			needsUpdate: false,
		},
		{
			subject1:    elasticUsers1,
			subject2:    elasticUsers3,
			needsUpdate: true,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.needsUpdate, tt.subject1.NeedsUpdate(tt.subject2.Secret()))
	}
}
