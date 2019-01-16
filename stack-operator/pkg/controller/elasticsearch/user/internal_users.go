package user

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/secret"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InternalUsers are Elasticsearch users intended for system use.
type InternalUsers struct {
	ControllerUser esclient.User
	KibanaUser     esclient.User
}

func NewInternalUsersFrom(users []esclient.User) InternalUsers {
	internalUsers := InternalUsers{}
	for _, user := range users {
		if user.Name == secret.InternalControllerUserName {
			internalUsers.ControllerUser = user
		}
		if user.Name == secret.InternalKibanaServerUserName {
			internalUsers.KibanaUser = user
		}
	}
	return internalUsers
}

// ReconcileUsers aggregates two clear-text secrets into an ES readable secret.
// The 'internal-users' secret contains credentials for use by other stack components like
// Kibana and for use by the controller or liveliness probes.
// The 'elastic-user' secret contains credentials for the reserved bootstrap user 'elastic'
// which needs to be known by users in order to be able to interact with the cluster.
// The aggregated secret is used to mount a 'users' file consisting of a sequence of username:bcrypt hashes
// into the Elasticsearch config directory which the file realm of ES security can directly understand.
// A second file called 'users_roles' is contained in this third secret as well which describes
// role assignments for the users specified in the first file.
func ReconcileUsers(
	c client.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
) (*InternalUsers, error) {

	internalSecrets := secret.NewInternalUserCredentials(es)

	if err := secret.ReconcileUserCredentialsSecret(c, scheme, es, internalSecrets); err != nil {
		return nil, err
	}

	users := internalSecrets.Users()
	internalUsers := NewInternalUsersFrom(users)
	externalSecrets := secret.NewExternalUserCredentials(es)

	if err := secret.ReconcileUserCredentialsSecret(c, scheme, es, externalSecrets); err != nil {
		return nil, err
	}

	for _, u := range externalSecrets.Users() {
		users = append(users, u)
	}

	elasticUsersSecret, err := secret.NewElasticUsersCredentials(es, users)
	if err != nil {
		return nil, err
	}

	if err := secret.ReconcileUserCredentialsSecret(c, scheme, es, elasticUsersSecret); err != nil {
		return nil, err
	}
	return &internalUsers, err
}
