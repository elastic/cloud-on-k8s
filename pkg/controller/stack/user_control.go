package stack

import (
	"context"

	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// InternalUsers are Elasticsearch intended for system use.
type InternalUsers struct {
	ControllerUser client.User
	KibanaUser     client.User
}

func NewInternalUsersFrom(users []client.User) InternalUsers {
	internalUsers := InternalUsers{}
	for _, user := range users {
		if user.Name == elasticsearch.InternalControllerUserName {
			internalUsers.ControllerUser = user
		}
		if user.Name == elasticsearch.InternalKibanaServerUserName {
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
func (r *ReconcileStack) reconcileUsers(stack *deploymentsv1alpha1.Stack) (InternalUsers, error) {

	internalSecrets := elasticsearch.NewInternalUserCredentials(*stack)

	err := r.reconcileSecret(stack, internalSecrets)
	if err != nil {
		return InternalUsers{}, err
	}

	users := internalSecrets.Users()
	internalUsers := NewInternalUsersFrom(users)
	externalSecrets := elasticsearch.NewExternalUserCredentials(*stack)

	err = r.reconcileSecret(stack, externalSecrets)
	if err != nil {
		return internalUsers, err
	}

	for _, u := range externalSecrets.Users() {
		users = append(users, u)
	}

	elasticUsersSecret, err := elasticsearch.NewElasticUsersCredentials(*stack, users)
	if err != nil {
		return internalUsers, err
	}
	err = r.reconcileSecret(stack, elasticUsersSecret)
	return internalUsers, err
}

// ReconcileSecret creates or updates the given credentials.
func (r *ReconcileStack) reconcileSecret(stack *deploymentsv1alpha1.Stack, expectedCreds elasticsearch.UserCredentials) error {
	expected := expectedCreds.Secret()
	if err := controllerutil.SetControllerReference(stack, &expected, r.scheme); err != nil {
		return err
	}
	found := &corev1.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info(common.Concat("Creating secret ", expected.Namespace, "/", expected.Name),
			"iteration", r.iteration,
		)
		return r.Create(context.TODO(), &expected)
	} else if err != nil {
		return err
	}

	if expectedCreds.NeedsUpdate(*found) {
		log.Info(
			common.Concat("Updating secret ", expected.Namespace, "/", expected.Name),
			"iteration", r.iteration,
		)
		found.Data = expected.Data // only update data, keep the rest
		err := r.Update(context.TODO(), found)
		if err != nil {
			return err
		}
	}
	expectedCreds.ResetTo(*found)
	return nil
}
