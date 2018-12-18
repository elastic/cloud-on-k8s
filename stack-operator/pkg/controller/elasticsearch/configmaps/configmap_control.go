package configmaps

import (
	"context"
	"reflect"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ReconcileConfigMap checks for an existing config map and updates it or creates one if it does not exist.
func ReconcileConfigMap(
	c client.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
	expected corev1.ConfigMap,
) error {
	if err := controllerutil.SetControllerReference(&es, &expected, scheme); err != nil {
		return err
	}

	found := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		log.Info("Creating config map", "namespace", expected.Namespace, "name", expected.Name)
		err = c.Create(context.TODO(), &expected)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// TODO proper comparison
	if !reflect.DeepEqual(expected.Data, found.Data) {
		log.Info("Updating config map", "namespace", expected.Namespace, "name", expected.Name)

		err := c.Update(context.TODO(), &expected)
		if err != nil {
			return err
		}
	}
	return nil

}
