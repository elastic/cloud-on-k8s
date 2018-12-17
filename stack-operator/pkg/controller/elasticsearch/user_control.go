package elasticsearch

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/reconciler"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	corev1 "k8s.io/api/core/v1"
)

// InternalUsers are Elasticsearch users intended for system use.
type InternalUsers struct {
	ControllerUser client.User
	KibanaUser     client.User
}

func NewInternalUsersFrom(users []client.User) InternalUsers {
	internalUsers := InternalUsers{}
	for _, user := range users {
		if user.Name == support.InternalControllerUserName {
			internalUsers.ControllerUser = user
		}
		if user.Name == support.InternalKibanaServerUserName {
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
func (r *ReconcileElasticsearch) reconcileUsers(es *v1alpha1.ElasticsearchCluster) (InternalUsers, error) {

	internalSecrets := support.NewInternalUserCredentials(*es)

	if err := r.reconcileSecret(es, internalSecrets); err != nil {
		return InternalUsers{}, err
	}

	users := internalSecrets.Users()
	internalUsers := NewInternalUsersFrom(users)
	externalSecrets := support.NewExternalUserCredentials(*es)

	if err := r.reconcileSecret(es, externalSecrets); err != nil {
		return internalUsers, err
	}

	for _, u := range externalSecrets.Users() {
		users = append(users, u)
	}

	elasticUsersSecret, err := support.NewElasticUsersCredentials(*es, users)
	if err != nil {
		return internalUsers, err
	}
	err = r.reconcileSecret(es, elasticUsersSecret)
	return internalUsers, err
}

// ReconcileSecret creates or updates the given credentials.
func (r *ReconcileElasticsearch) reconcileSecret(es *v1alpha1.ElasticsearchCluster, expectedCreds support.UserCredentials) error {
	expected := expectedCreds.Secret()
	err := reconciler.ReconcileResource(reconciler.Params{
		Client: r,
		Scheme: r.scheme,
		Owner:  es,
		Object: &expected,
		Differ: func(_, found *corev1.Secret) bool {
			return expectedCreds.NeedsUpdate(*found)
		},
		Modifier: func(expected, found *corev1.Secret) {
			found.Data = expected.Data // only update data, keep the rest
		},
	})
	if err != nil {
		//expected creds have been updated to reflect the state on the API server
		expectedCreds.Reset(expected)
	}
	return err
}
