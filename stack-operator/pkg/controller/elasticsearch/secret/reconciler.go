package secret

import (
	"context"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("secret")
)

// ReconcileSecret creates or updates the given credentials.
func ReconcileUserCredentialsSecret(
	c client.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
	creds support.UserCredentials,
) error {
	expected := creds.Secret()
	if err := controllerutil.SetControllerReference(&es, &expected, scheme); err != nil {
		return err
	}
	found := &v1.Secret{}
	err := c.Get(context.TODO(), k8s.ExtractNamespacedName(expected.ObjectMeta), found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating secret", "namespace", expected.Namespace, "name", expected.Name)
		return c.Create(context.TODO(), &expected)
	} else if err != nil {
		return err
	}

	if creds.NeedsUpdate(*found) {
		log.Info("Updating secret", "namespace", expected.Namespace, "name", expected.Name)
		found.Data = expected.Data // only update data, keep the rest
		err := c.Update(context.TODO(), found)
		if err != nil {
			return err
		}
	}
	creds.Reset(*found)
	return nil
}
