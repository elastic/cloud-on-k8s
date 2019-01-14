package driver

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	esnodecerts "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// reconcileNodeCertificates ensures that the CA cert is pushed to the API and node certificates are issued.
func reconcileNodeCertificates(
	c client.Client,
	scheme *runtime.Scheme,
	ca *nodecerts.Ca,
	es v1alpha1.ElasticsearchCluster,
	services []corev1.Service,
) error {
	// TODO: suffix with type (-ca?) and trim
	clusterCAPublicSecretObjectKey := k8s.ExtractNamespacedName(es.ObjectMeta)
	if err := ca.ReconcilePublicCertsSecret(c, clusterCAPublicSecretObjectKey, &es, scheme); err != nil {
		return err
	}

	// reconcile node certificates since we might have new pods (or existing pods that needs a refresh)
	if _, err := esnodecerts.ReconcileNodeCertificateSecrets(c, ca, es, services); err != nil {
		return err
	}

	return nil
}
