package license

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/watches"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconcile reconciles the current Elasticsearch license with the desired one.
func Reconcile(
	c client.Client,
	w watches.DynamicWatches,
	esCluster types.NamespacedName,
	clusterClient *esclient.Client,
	current *esclient.License,
) error {
	if err := ensureLicenseWatch(esCluster, w); err != nil {
		return err
	}
	return applyLinkedLicense(c, esCluster, clusterClient, current)
}
