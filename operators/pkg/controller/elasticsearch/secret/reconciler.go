package secret

import (
	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/stack-operator/pkg/utils/k8s"

	"github.com/elastic/k8s-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("secret")
)

// ReconcileSecret creates or updates the given credentials.
func ReconcileUserCredentialsSecret(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
	creds UserCredentials,
) error {
	expected := creds.Secret()
	reconciled := &corev1.Secret{}
	err := reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme,
		Owner:      &es,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			return creds.NeedsUpdate(*reconciled)
		},
		UpdateReconciled: func() {
			reconciled.Data = expected.Data // only update data, keep the rest
		},
	})
	if err == nil {
		// expected creds have been updated to reflect the state on the API server
		creds.Reset(*reconciled)
	}
	return err
}
