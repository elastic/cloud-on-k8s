package stack

import (
	"context"
	"reflect"

	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// ReconcileUsers aggregates secrets into a ES readable secret.
func (r *ReconcileStack) reconcileUsers(stack *deploymentsv1alpha1.Stack) (client.User, error) {

	internalUser := client.User{}
	//TODO watch secrets
	expected := elasticsearch.NewInternalUserSecret(*stack)
	internalSecrets := &corev1.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, internalSecrets)
	if err != nil && errors.IsNotFound(err) {
		log.Info(common.Concat("Creating secret ", expected.Namespace, "/", expected.Name),
			"iteration", r.iteration,
		)
		err = r.Create(context.TODO(), &expected)
		if err != nil {
			return internalUser, err
		}
		internalSecrets = &expected
	} else if err != nil {
		return internalUser, err
	}

	users := elasticsearch.NewUsersFromSecret(*internalSecrets)
	for _, user := range users {
		if user.Name == elasticsearch.InternalUserName {
			internalUser = user
		}
	}

	elasticUsersSecret, err := elasticsearch.NewElasticUsersSecret(*stack, users)
	if err != nil {
		return internalUser, err
	}
	err = r.reconcileSecret(stack, elasticUsersSecret)
	return internalUser, err
}

//ReconcileSecret creates or updates the a given secret.
func (r *ReconcileStack) reconcileSecret(stack *deploymentsv1alpha1.Stack, expected corev1.Secret) error {
	found := &corev1.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info(common.Concat("Creating secret ", expected.Namespace, "/", expected.Name),
			"iteration", r.iteration,
		)
		err = r.Create(context.TODO(), &expected)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if !reflect.DeepEqual(expected.Data, found.Data) {
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
	return nil
}
