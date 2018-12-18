package driver

import (
	"crypto/x509"
	"fmt"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/version"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/reconcilehelpers"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/snapshots"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/users"
	esversion "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/net"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Driver interface {
	Reconcile(
		es v1alpha1.ElasticsearchCluster,
		state *reconcilehelpers.ReconcileState,
	) *reconcilehelpers.ReconcileResults
}

type Options struct {
	Version version.Version
	Client  client.Client
	Scheme  *runtime.Scheme

	ClusterCa *nodecerts.Ca
	Dialer    net.Dialer
}

// NewDriver returns a Driver that can operate the provided version
func NewDriver(opts Options) (Driver, error) {
	versionStrategy, err := esversion.LookupStrategy(opts.Version)
	if err != nil {
		return nil, err
	}

	driver := &strategyDriver{
		opts:            opts,
		versionStrategy: versionStrategy,

		genericResourcesReconciler: ReconcileGenericResources,
		nodeCertificatesReconciler: ReconcileNodeCertificates,

		observedStateResolver:  support.NewObservedState,
		resourcesStateResolver: support.NewResourcesStateFromAPI,
		usersResolver:          users.ReconcileUsers,
	}

	return driver, nil
}

type strategyDriver struct {
	opts Options

	versionStrategy esversion.ElasticsearchVersionStrategy

	genericResourcesReconciler func(
		c client.Client,
		scheme *runtime.Scheme,
		es v1alpha1.ElasticsearchCluster,
	) (*GenericResources, error)

	nodeCertificatesReconciler func(
		c client.Client,
		scheme *runtime.Scheme,
		ca *nodecerts.Ca,
		es v1alpha1.ElasticsearchCluster,
		services []v1.Service,
	) error

	usersResolver func(
		c client.Client,
		scheme *runtime.Scheme,
		es v1alpha1.ElasticsearchCluster,
	) (*users.InternalUsers, error)

	//versionWideResourcesReconciler interface{} // version-specific

	//expectedPodsAndResourcesResolver interface{} // version-specific, uses common tooling (mostly from version package)

	observedStateResolver func(esClient *esclient.Client) support.ObservedState

	resourcesStateResolver func(
		c client.Client,
		es v1alpha1.ElasticsearchCluster,
	) (*support.ResourcesState, error)

	//discoverySettingsUpdater interface{} // likely only one impl for now, 7.0 will force a change here

	//performableChangesResolver interface{} // only one impl

	//changeReconciler interface{} // only one impl, but composed of version-specific components

	//dataMigrator func(esClient *esclient.Client, leavingNodeNames []string) error // varies by version

	//apiObjectsGarbageCollector func(
	//	c client.Client,
	//	es v1alpha1.ElasticsearchCluster,
	//	version version.Version,
	//	state mutation.PodsState,
	//) (reconcile.Result, error) // could get away with one impl
}

func (d *strategyDriver) Reconcile(
	es v1alpha1.ElasticsearchCluster,
	reconcileState *reconcilehelpers.ReconcileState,
) *reconcilehelpers.ReconcileResults {
	results := &reconcilehelpers.ReconcileResults{}

	genericResources, err := d.genericResourcesReconciler(d.opts.Client, d.opts.Scheme, es)
	if err != nil {
		return results.WithError(err)
	}

	if err := d.nodeCertificatesReconciler(
		d.opts.Client,
		d.opts.Scheme,
		d.opts.ClusterCa,
		es,
		[]v1.Service{genericResources.PublicService, genericResources.DiscoveryService},
	); err != nil {
		return results.WithError(err)
	}

	internalUsers, err := d.usersResolver(d.opts.Client, d.opts.Scheme, es)
	if err != nil {
		return results.WithError(err)
	}

	esClient := d.newElasticsearchClient(genericResources.PublicService, internalUsers.ControllerUser)

	observedState := d.observedStateResolver(esClient)

	resourcesState, err := d.resourcesStateResolver(d.opts.Client, es)
	if err != nil {
		return results.WithError(err)
	}

	// always update the elasticsearch state bits
	if observedState.ClusterState != nil && observedState.ClusterHealth != nil {
		reconcileState.UpdateElasticsearchState(*resourcesState, observedState)
	}

	podsState := mutation.NewPodsState(*resourcesState, observedState)

	// recoverable reconcile steps start here. In case of error we record the error and continue
	results.Apply("reconcileElasticsearchPods", func() (reconcile.Result, error) {
		return d.reconcileElasticsearchPods(
			d.opts.Client,
			d.opts.Scheme,
			d.opts.ClusterCa,
			es,
			genericResources.PublicService,
			esClient,
			podsState,
			reconcileState,
			*resourcesState,
			observedState,
			podsState,
			d.versionStrategy,
			internalUsers.ControllerUser,
		)
	})

	if err := snapshots.ReconcileSnapshotterCronJob(
		d.opts.Client,
		d.opts.Scheme,
		es,
		internalUsers.ControllerUser,
	); err != nil {
		return results.WithError(err)
	}

	return results
}

// newElasticsearchClient creates a new Elasticsearch HTTP client for this cluster using the provided user
func (d *strategyDriver) newElasticsearchClient(service v1.Service, user esclient.User) *esclient.Client {
	certPool := x509.NewCertPool()
	certPool.AddCert(d.opts.ClusterCa.Cert)

	url := fmt.Sprintf("https://%s.%s.svc.cluster.local:%d", service.Name, service.Namespace, support.HTTPPort)

	esClient := esclient.NewElasticsearchClient(
		d.opts.Dialer, url, user, certPool,
	)
	return esClient
}
