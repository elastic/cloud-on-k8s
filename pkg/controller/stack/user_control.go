package stack

import (
	"context"
	"reflect"

	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

type InternalUsers struct {
	ControllerUser client.User
	KibanaUser     client.User
}

// ReconcileUsers aggregates secrets into a ES readable secret.
func (r *ReconcileStack) reconcileUsers(stack *deploymentsv1alpha1.Stack) (InternalUsers, error) {

	internalUsers := InternalUsers{}
	internalSecrets := elasticsearch.NewInternalUserSecret(*stack)
	err := r.reconcileSecret(stack, &internalSecrets, true)
	if err != nil {
		return internalUsers, err
	}

	users := elasticsearch.NewUsersFromSecret(internalSecrets)
	for _, user := range users {
		if user.Name == elasticsearch.InternalControllerUserName {
			internalUsers.ControllerUser = user
		}
		if user.Name == elasticsearch.InternalKibanaServerUserName {
			internalUsers.KibanaUser = user
		}
	}

	externalSecrets := elasticsearch.NewExternalUserSecret(*stack)

	err = r.reconcileSecret(stack, &externalSecrets, true)
	if err != nil {
		return internalUsers, err
	}

	for _, u := range elasticsearch.NewUsersFromSecret(externalSecrets) {
		users = append(users, u)
	}

	elasticUsersSecret, err := elasticsearch.NewElasticUsersSecret(*stack, users)
	if err != nil {
		return internalUsers, err
	}
	err = r.reconcileSecret(stack, &elasticUsersSecret, false)
	return internalUsers, err
}

func keysEqual(v1, v2 map[string][]byte) bool {
	if len(v1) != len(v2) {
		return false
	}

	for k, _ := range v1 {
		if _, ok := v2[k]; !ok {
			return false
		}
	}
	return true
}

// ReconcileSecret creates or updates the a given secret.
// Use keyPresenceOnly to avoid overwriting randomly generated secrets unnecessarily.
func (r *ReconcileStack) reconcileSecret(stack *deploymentsv1alpha1.Stack, expected *corev1.Secret, keyPresenceOnly bool) error {
	if err := controllerutil.SetControllerReference(stack, expected, r.scheme); err != nil {
		return err
	}
	found := &corev1.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info(common.Concat("Creating secret ", expected.Namespace, "/", expected.Name),
			"iteration", r.iteration,
		)
		err = r.Create(context.TODO(), expected)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	var updateNeeded bool
	if keyPresenceOnly {
		// for generated secrets as long as the key exists we can work with it. Rotate secrets by deleting them (?)
		updateNeeded = !keysEqual(expected.Data, found.Data)
	} else {
		// TODO this will trigger everytime because of bcrypt, be smarter here and check the bcrypt hash
		updateNeeded = !reflect.DeepEqual(expected.Data, found.Data)
	}

	if updateNeeded {
		log.Info(
			common.Concat("Updating secret ", expected.Namespace, "/", expected.Name),
			"iteration", r.iteration,
		)
		found.Data = expected.Data // only update data, keep the rest
		err := r.Update(context.TODO(), found)
		if err != nil {
			return err
		}
	} else if keyPresenceOnly {
		expected.Data = found.Data //make sure expected reflects the state on the API server
	}
	return nil
}
