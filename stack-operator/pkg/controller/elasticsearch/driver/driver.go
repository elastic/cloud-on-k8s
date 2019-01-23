package driver

import (
	"fmt"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/watches"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/observer"
	esreconcile "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/user"
	esversion "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/version/version5"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/version/version6"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/version/version7"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/net"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("driver")

	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// Driver is something that can reconcile an ElasticsearchCluster resource
type Driver interface {
	Reconcile(
		es v1alpha1.ElasticsearchCluster,
		reconcileState *esreconcile.State,
	) *esreconcile.Results
}

// Options are used to create a driver. See NewDriver
type Options struct {
	// Version is the version of Elasticsearch we want to reconcile towards
	Version version.Version
	// Client is used to access the Kubernetes API
	Client client.Client
	Scheme *runtime.Scheme

	// ClusterCa is the CA that is used to issue certificates for nodes in the cluster
	ClusterCa *nodecerts.Ca
	// Dialer is used to create the Elasticsearch HTTP client.
	Dialer net.Dialer
	// Observers that observe es clusters state
	Observers *observer.Manager
	// DynamicWatches are handles to currently registered dynamic watches.
	DynamicWatches watches.DynamicWatches
}

// NewDriver returns a Driver that can operate the provided version
func NewDriver(opts Options) (Driver, error) {
	driver := &defaultDriver{
		Options: opts,

		genericResourcesReconciler: reconcileGenericResources,
		nodeCertificatesReconciler: reconcileNodeCertificates,

		versionWideResourcesReconciler: reconcileVersionWideResources,

		observedStateResolver:   opts.Observers.ObservedStateResolver,
		resourcesStateResolver:  esreconcile.NewResourcesStateFromAPI,
		internalUsersReconciler: user.ReconcileUsers,
	}

	switch opts.Version.Major {
	case 7:
		driver.expectedPodsAndResourcesResolver = version6.ExpectedPodSpecs

		driver.clusterInitialMasterNodesEnforcer = version7.ClusterInitialMasterNodesEnforcer

		// version 7 uses zen2 instead of zen
		driver.zen2SettingsUpdater = version7.UpdateZen2Settings
		// .. except we still have to manage minimum_master_nodes while doing a rolling upgrade from 6 -> 7
		// we approximate this by also handling zen 1, even in 7
		// TODO: only do this if there's 6.x masters in the cluster.
		driver.zen1SettingsUpdater = esversion.UpdateZen1Discovery

		driver.supportedVersions = esversion.LowestHighestSupportedVersions{
			// 6.6.0 is the lowest wire compatibility version for 7.x
			LowestSupportedVersion: version.MustParse("6.6.0"),
			// higher may be possible, but not proven yet, lower may also be a requirement...
			HighestSupportedVersion: version.MustParse("7.0.99"),
		}

	case 6:
		driver.expectedPodsAndResourcesResolver = version6.ExpectedPodSpecs
		driver.zen1SettingsUpdater = esversion.UpdateZen1Discovery
		driver.supportedVersions = esversion.LowestHighestSupportedVersions{
			// 5.6.0 is the lowest wire compatibility version for 6.x
			LowestSupportedVersion: version.MustParse("5.6.0"),
			// higher may be possible, but not proven yet, lower may also be a requirement...
			HighestSupportedVersion: version.MustParse("6.4.99"),
		}
	case 5:
		driver.expectedPodsAndResourcesResolver = version5.ExpectedPodSpecs
		driver.zen1SettingsUpdater = esversion.UpdateZen1Discovery
		driver.supportedVersions = esversion.LowestHighestSupportedVersions{
			// TODO: verify that we actually support down to 5.0.0
			// TODO: this follows ES version compat, which is wrong, because we would have to be able to support
			//       an elasticsearch cluster full of 2.x (2.4.6 at least) instances which we would probably only want
			// 		 to do upgrade checks on, snapshot, then terminate + snapshot restore (or re-use volumes).
			LowestSupportedVersion: version.MustParse("5.0.0"),
			// higher may be possible, but not proven yet, lower may also be a requirement...
			HighestSupportedVersion: version.MustParse("5.6.99"),
		}
	default:
		return nil, fmt.Errorf("unsupported version: %s", opts.Version)
	}

	return driver, nil
}
