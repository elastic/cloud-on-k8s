package stack

import (
	"context"
	"reflect"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileStack) reconcileUsers(stack *deploymentsv1alpha1.Stack) (reconcile.Result, error) {

	//TODO watch secrets
	expected := elasticsearch.NewInternalUserSecret(*stack)
	internalSecrets := &v1.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, internalSecrets)
	if err != nil && errors.IsNotFound(err) {
		log.Info(common.Concat("Creating secret ", expected.Namespace, "/", expected.Name),
			"iteration", r.iteration,
		)
		err = r.Create(context.TODO(), &expected)
		if err != nil {
			return reconcile.Result{}, err
		}
		internalSecrets = &expected
	} else if err != nil {
		return reconcile.Result{}, err
	}

	users, err := elasticsearch.NewUsersFromSecret(*internalSecrets)
	if err != nil {
		return reconcile.Result{}, err
	}

	elasticUsersSecret, err := elasticsearch.NewElasticUsersSecret(*stack, users)
	if err != nil {
		return reconcile.Result{}, err
	}
	return r.reconcileSecret(stack, elasticUsersSecret)
}

func (r *ReconcileStack) reconcileSecret(stack *deploymentsv1alpha1.Stack, expected v1.Secret) (reconcile.Result, error) {
	found := &v1.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info(common.Concat("Creating secret ", expected.Namespace, "/", expected.Name),
			"iteration", r.iteration,
		)
		err = r.Create(context.TODO(), &expected)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else if err != nil {
		return reconcile.Result{}, err
	}

	if !reflect.DeepEqual(expected.Data, found.Data) {
		log.Info(
			common.Concat("Updating secret ", expected.Namespace, "/", expected.Name),
			"iteration", r.iteration,
		)
		found.Data = expected.Data // only update data, keep the rest
		err := r.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}
