package driver

import (
	"context"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/events"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/version"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/reconcilehelper"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/services"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/snapshot"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/user"
	esversion "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/net"
	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
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
		state *reconcilehelper.ReconcileState,
	) *reconcilehelper.ReconcileResults
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
}

// NewDriver returns a Driver that can operate the provided version
func NewDriver(opts Options) (Driver, error) {
	driver := &strategyDriver{
		Options: opts,

		genericResourcesReconciler: reconcileGenericResources,
		nodeCertificatesReconciler: reconcileNodeCertificates,

		versionWideResourcesReconciler: reconcileVersionWideResources,

		observedStateResolver:   support.NewObservedState,
		resourcesStateResolver:  support.NewResourcesStateFromAPI,
		internalUsersReconciler: user.ReconcileUsers,
	}

	switch opts.Version.Major {
	case 7:
		// TODO: handle differences from 6
		driver.expectedPodsAndResourcesResolver = esversion.ExpectedPodSpecs6
		driver.podBuilder = esversion.NewPod6
		// TODO: zen 2?
		driver.discoverySettingsUpdater = esversion.UpdateZen1Discovery
		driver.supportedVersions = esversion.LowestHighestSupportedVersions{
			// 6.6.0 is the lowest wire compatibility version for 7.x
			LowestSupportedVersion: version.MustParse("6.6.0"),
			// higher may be possible, but not proven yet, lower may also be a requirement...
			HighestSupportedVersion: version.MustParse("7.0.99"),
		}

	case 6:
		driver.expectedPodsAndResourcesResolver = esversion.ExpectedPodSpecs6
		driver.podBuilder = esversion.NewPod6
		driver.discoverySettingsUpdater = esversion.UpdateZen1Discovery
		driver.supportedVersions = esversion.LowestHighestSupportedVersions{
			// 5.6.0 is the lowest wire compatibility version for 6.x
			LowestSupportedVersion: version.MustParse("5.6.0"),
			// higher may be possible, but not proven yet, lower may also be a requirement...
			HighestSupportedVersion: version.MustParse("6.4.99"),
		}
	case 5:
		driver.expectedPodsAndResourcesResolver = esversion.ExpectedPodSpecs5
		driver.podBuilder = esversion.NewPod5
		driver.discoverySettingsUpdater = esversion.UpdateZen1Discovery
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

// strategyDriver is the default Driver implementation
type strategyDriver struct {
	// Options are the options that the driver was created with.
	Options

	// supportedVersions verified whether we can support upgrading from the current pods.
	supportedVersions esversion.LowestHighestSupportedVersions

	// genericResourcesReconciler reconciles non-version specific resources.
	genericResourcesReconciler func(
		c client.Client,
		scheme *runtime.Scheme,
		es v1alpha1.ElasticsearchCluster,
	) (*GenericResources, error)

	// nodeCertificatesReconciler reconciles node certificates
	nodeCertificatesReconciler func(
		c client.Client,
		scheme *runtime.Scheme,
		ca *nodecerts.Ca,
		es v1alpha1.ElasticsearchCluster,
		services []v1.Service,
	) error

	// internalUsersReconciler reconciles and returns the current internal users.
	internalUsersReconciler func(
		c client.Client,
		scheme *runtime.Scheme,
		es v1alpha1.ElasticsearchCluster,
	) (*user.InternalUsers, error)

	// versionWideResourcesReconciler reconciles resources that may be specific to a version
	versionWideResourcesReconciler func(
		c client.Client,
		scheme *runtime.Scheme,
		es v1alpha1.ElasticsearchCluster,
	) (*VersionWideResources, error)

	// expectedPodsAndResourcesResolver returns a list of pod specs with context that we would expect to find in the
	// Elasticsearch cluster.
	//
	// paramsTmpl argument is a partially filled NewPodSpecParams (TODO: refactor into its own params struct)
	expectedPodsAndResourcesResolver func(
		es v1alpha1.ElasticsearchCluster,
		paramsTmpl support.NewPodSpecParams,
	) ([]support.PodSpecContext, error)

	// podBuilder constructs Pod objects before creation
	podBuilder func(
		version version.Version,
		es v1alpha1.ElasticsearchCluster,
		podSpecCtx support.PodSpecContext,
	) (v1.Pod, error)

	// observedStateResolver resolves the currently observed state of Elasticsearch from the ES API
	observedStateResolver func(esClient *esclient.Client) support.ObservedState

	// resourcesStateResolver resolves the current state of the K8s resources from the K8s API
	resourcesStateResolver func(
		c client.Client,
		es v1alpha1.ElasticsearchCluster,
	) (*support.ResourcesState, error)

	// discoverySettingsUpdater updates the discovery settings for the current pods.
	discoverySettingsUpdater func(esClient *esclient.Client, allPods []v1.Pod) error

	// TODO: implement
	//// apiObjectsGarbageCollector garbage collects API objects for older versions once they are no longer needed.
	//apiObjectsGarbageCollector func(
	//	c client.Client,
	//	es v1alpha1.ElasticsearchCluster,
	//	version version.Version,
	//	state mutation.PodsState,
	//) (reconcile.Result, error) // could get away with one impl
}

// Reconcile fulfills the Driver interface and reconciles the cluster resources.
func (d *strategyDriver) Reconcile(
	es v1alpha1.ElasticsearchCluster,
	reconcileState *reconcilehelper.ReconcileState,
) *reconcilehelper.ReconcileResults {
	results := &reconcilehelper.ReconcileResults{}

	genericResources, err := d.genericResourcesReconciler(d.Client, d.Scheme, es)
	if err != nil {
		return results.WithError(err)
	}

	if err := d.nodeCertificatesReconciler(
		d.Client,
		d.Scheme,
		d.ClusterCa,
		es,
		[]v1.Service{genericResources.PublicService, genericResources.DiscoveryService},
	); err != nil {
		return results.WithError(err)
	}

	internalUsers, err := d.internalUsersReconciler(d.Client, d.Scheme, es)
	if err != nil {
		return results.WithError(err)
	}

	esClient := d.newElasticsearchClient(genericResources.PublicService, internalUsers.ControllerUser)

	observedState := d.observedStateResolver(esClient)

	resourcesState, err := d.resourcesStateResolver(d.Client, es)
	if err != nil {
		return results.WithError(err)
	}

	// always update the elasticsearch state bits
	if observedState.ClusterState != nil && observedState.ClusterHealth != nil {
		reconcileState.UpdateElasticsearchState(*resourcesState, observedState)
	}

	podsState := mutation.NewPodsState(*resourcesState, observedState)

	if err := d.supportedVersions.VerifySupportsExistingPods(resourcesState.CurrentPods); err != nil {
		return results.WithError(err)
	}

	versionWideResources, err := d.versionWideResourcesReconciler(d.Client, d.Scheme, es)
	if err != nil {
		return results.WithError(err)
	}

	if err := snapshot.ReconcileSnapshotterCronJob(
		d.Client,
		d.Scheme,
		es,
		internalUsers.ControllerUser,
	); err != nil {
		// it's ok to continue even if we cannot reconcile the cron job
		results.WithError(err)
	}

	changes, err := d.calculateChanges(versionWideResources, internalUsers, es, resourcesState)
	if err != nil {
		return results.WithError(err)
	}

	log.Info(
		"Going to apply the following topology changes",
		"ToCreate:", len(changes.ToCreate),
		"ToKeep:", len(changes.ToKeep),
		"ToDelete:", len(changes.ToDelete),
	)

	// figure out what changes we can perform right now
	performableChanges, err := mutation.CalculatePerformableChanges(
		es.Spec.UpdateStrategy,
		changes,
		podsState,
	)
	if err != nil {
		return results.WithError(err)
	}

	log.Info(
		"Calculated performable changes",
		"schedule_for_creation_count", len(performableChanges.ToCreate),
		"schedule_for_deletion_count", len(performableChanges.ToDelete),
	)

	esReachable, err := services.IsServiceReady(d.Client, genericResources.PublicService)
	if err != nil {
		return results.WithError(err)
	}

	if esReachable { // TODO this needs to happen outside of reconcileElasticsearchPods pending refactoring
		err = snapshot.EnsureSnapshotRepository(context.TODO(), esClient, es.Spec.SnapshotRepository)
		if err != nil {
			// TODO decide should this be a reason to stop this reconciliation loop?
			msg := "Could not ensure snapshot repository"
			reconcileState.AddEvent(v1.EventTypeWarning, events.EventReasonUnexpected, msg)
			log.Error(err, msg)
		}
	}

	for _, change := range performableChanges.ToCreate {
		if err := createElasticsearchPod(
			d.Client,
			d.Scheme,
			es,
			reconcileState,
			change.Pod,
			change.PodSpecCtx,
		); err != nil {
			return results.WithError(err)
		}
	}

	if !changes.HasChanges() {
		// Current state matches expected state
		if !esReachable {
			// es not yet reachable, let's try again later.
			return results.WithResult(defaultRequeue)
		}

		// Update discovery for any previously created pods that have come up (see also below in create pod)
		if err := d.discoverySettingsUpdater(
			esClient,
			reconcilehelper.AvailableElasticsearchNodes(resourcesState.CurrentPods),
		); err != nil {
			// TODO: reconsider whether this error should be propagated with results instead?
			log.Error(err, "Error during update discovery after having no changes, requeuing.")
			return results.WithResult(defaultRequeue)
		}

		reconcileState.UpdateElasticsearchOperational(*resourcesState, observedState)
		return results
	}

	if !esReachable {
		// We cannot manipulate ES allocation exclude settings if the ES cluster
		// cannot be reached, hence we cannot delete pods.
		// Probably it was just created and is not ready yet.
		// Let's retry in a while.
		log.Info("ES public service not ready yet for shard migration reconciliation. Requeuing.")

		reconcileState.UpdateElasticsearchPending(resourcesState.CurrentPods)

		return results.WithResult(defaultRequeue)
	}

	// Start migrating data away from all pods to be deleted
	leavingNodeNames := support.PodListToNames(performableChanges.ToDelete)
	if err = support.MigrateData(esClient, leavingNodeNames); err != nil {
		return results.WithError(errors.Wrap(err, "error during migrate data"))
	}

	newState := make([]v1.Pod, len(resourcesState.CurrentPods))
	copy(newState, resourcesState.CurrentPods)

	// Shrink clusters by deleting deprecated pods
	for _, pod := range performableChanges.ToDelete {
		newState = removePodFromList(newState, pod)
		preDelete := func() error {
			return d.discoverySettingsUpdater(esClient, newState)
		}
		result, err := deleteElasticsearchPod(
			d.Client,
			reconcileState,
			*resourcesState,
			observedState,
			pod,
			performableChanges.ToDelete,
			preDelete,
		)
		if err != nil {
			return results.WithError(err)
		}
		results.WithResult(result)
	}
	if changes.HasChanges() && !performableChanges.HasChanges() {
		// if there are changes we'd like to perform, but none that were performable, we try again later
		results.WithResult(defaultRequeue)
	}

	reconcileState.UpdateElasticsearchState(*resourcesState, observedState)

	return results
}

// removePodFromList removes a single pod from the list, matching by pod name.
func removePodFromList(pods []v1.Pod, pod v1.Pod) []v1.Pod {
	for i, p := range pods {
		if p.Name == pod.Name {
			return append(pods[:i], pods[i+1:]...)
		}
	}
	return pods
}

// calculateChanges calculates the changes we'd need to perform to go from the current cluster configuration to the
// desired one.
func (d *strategyDriver) calculateChanges(
	versionWideResources *VersionWideResources,
	internalUsers *user.InternalUsers,
	es v1alpha1.ElasticsearchCluster,
	resourcesState *support.ResourcesState,
) (*mutation.Changes, error) {
	expectedPodSpecCtxs, err := d.expectedPodsAndResourcesResolver(
		es,
		support.NewPodSpecParams{
			ExtraFilesRef:   k8s.ExtractNamespacedName(versionWideResources.ExtraFilesSecret.ObjectMeta),
			KeystoreConfig:  versionWideResources.KeyStoreConfig,
			ProbeUser:       internalUsers.ControllerUser,
			ConfigMapVolume: support.NewConfigMapVolume(versionWideResources.GenericUnecryptedConfigurationFiles.Name, support.ManagedConfigPath),
		},
	)
	if err != nil {
		return nil, err
	}

	changes, err := mutation.CalculateChanges(
		expectedPodSpecCtxs,
		*resourcesState,
		func(ctx support.PodSpecContext) (v1.Pod, error) {
			return d.podBuilder(d.Version, es, ctx)
		},
	)
	if err != nil {
		return nil, err
	}
	return &changes, nil
}

// newElasticsearchClient creates a new Elasticsearch HTTP client for this cluster using the provided user
func (d *strategyDriver) newElasticsearchClient(service v1.Service, user esclient.User) *esclient.Client {
	certPool := x509.NewCertPool()
	certPool.AddCert(d.ClusterCa.Cert)

	url := fmt.Sprintf("https://%s.%s.svc.cluster.local:%d", service.Name, service.Namespace, support.HTTPPort)

	esClient := esclient.NewElasticsearchClient(
		d.Dialer, url, user, certPool,
	)
	return esClient
}
