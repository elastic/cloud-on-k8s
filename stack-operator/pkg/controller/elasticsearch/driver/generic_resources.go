package driver

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/services"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GenericResources are resources that all clusters have.
type GenericResources struct {
	// PublicService is the user-facing service
	PublicService corev1.Service
	// DiscoveryService is the service used by ES for discovery purposes
	DiscoveryService corev1.Service
}

// reconcileGenericResources reconciles the expected generic resources of a cluster.
func reconcileGenericResources(
	c client.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
) (*GenericResources, error) {
	// TODO: these reconciles do not necessarily use the services as in-out params.
	// TODO: consider removing the "res" bits of the ReconcileService signature?

	discoveryService := services.NewDiscoveryService(es)
	_, err := common.ReconcileService(c, scheme, discoveryService, &es)
	if err != nil {
		return nil, err
	}

	publicService := services.NewPublicService(es)
	_, err = common.ReconcileService(c, scheme, publicService, &es)
	if err != nil {
		return nil, err
	}

	return &GenericResources{DiscoveryService: *discoveryService, PublicService: *publicService}, nil
}
