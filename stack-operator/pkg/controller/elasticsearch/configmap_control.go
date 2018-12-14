package elasticsearch

import (
	"context"
	"reflect"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ReconcileConfigMap checks for an existing config map and updates it or creates one if it does not exist.
func (r *ReconcileElasticsearch) ReconcileConfigMap(es v1alpha1.ElasticsearchCluster, expected corev1.ConfigMap) error {
	if err := controllerutil.SetControllerReference(&es, &expected, r.scheme); err != nil {
		return err
	}

	found := &corev1.ConfigMap{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		log.Info(common.Concat("Creating config map ", expected.Namespace, "/", expected.Name),
			"iteration", r.iteration,
		)
		err = r.Create(context.TODO(), &expected)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// TODO proper comparison
	if !reflect.DeepEqual(expected.Data, found.Data) {
		log.Info(
			common.Concat("Updating config map ", expected.Namespace, "/", expected.Name),
			"iteration", r.iteration,
		)
		err := r.Update(context.TODO(), &expected)
		if err != nil {
			return err
		}
	}
	return nil

}
