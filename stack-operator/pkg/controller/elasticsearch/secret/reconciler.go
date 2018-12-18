package secret

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/reconciler"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	err := reconciler.ReconcileResource(reconciler.Params{
		Client: c,
		Scheme: scheme,
		Owner:  &es,
		Object: &expected,
		Differ: func(_, found *v1.Secret) bool {
			return creds.NeedsUpdate(*found)
		},
		Modifier: func(expected, found *v1.Secret) {
			found.Data = expected.Data // only update data, keep the rest
		},
	})
	if err == nil {
		//expected creds have been updated to reflect the state on the API server
		creds.Reset(expected)
	}
	return err
}
