package elasticsearch

import (
	"sort"
	"testing"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewUserSecrets(t *testing.T) {
	mockStack := deploymentsv1alpha1.Stack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-stack",
			Namespace: "default",
		}}

	elasticUsers, err := NewElasticUsersSecret(mockStack, []client.User{client.User{Name: "foo", Password: "bar"}})
	assert.NoError(t, err)

	tests := []struct {
		subject      corev1.Secret
		expectedName string
		expectedKes  []string
	}{
		{
			subject:      NewInternalUserSecret(mockStack),
			expectedName: "my-stack-internal-users",
			expectedKes:  []string{InternalControllerUserName, InternalKibanaServerUserName},
		},
		{
			subject:      NewExternalUserSecret(mockStack),
			expectedName: "my-stack-elastic-user",
			expectedKes:  []string{ExternalUserName},
		},
		{
			subject:      elasticUsers,
			expectedName: "my-stack-users",
			expectedKes:  []string{ElasticUsers, ElasticUsersRoles},
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expectedName, tt.subject.Name)
		var keys []string
		for k, _ := range tt.subject.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		assert.EqualValues(t, tt.expectedKes, keys)
	}

}
