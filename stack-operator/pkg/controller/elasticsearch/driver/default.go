package driver

import (
	"context"
	"crypto/x509"
	"fmt"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/events"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/watches"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/license"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/migration"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/observer"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/pod"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/services"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/settings"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/snapshot"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/user"
	esversion "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/volume"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controller "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// defaultDriver is the default Driver implementation
type defaultDriver struct {
	// Options are the options that the driver was created with.
	Options

	// supportedVersions verifies whether we can support upgrading from the current pods.
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
		services []corev1.Service,
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
		w watches.DynamicWatches,
	) (*VersionWideResources, error)

	// expectedPodsAndResourcesResolver returns a list of pod specs with context that we would expect to find in the
	// Elasticsearch cluster.
	//
	// paramsTmpl argument is a partially filled NewPodSpecParams (TODO: refactor into its own params struct)
	expectedPodsAndResourcesResolver func(
		es v1alpha1.ElasticsearchCluster,
		paramsTmpl pod.NewPodSpecParams,
		operatorImage string,
	) ([]pod.PodSpecContext, error)

	// observedStateResolver resolves the currently observed state of Elasticsearch from the ES API
	observedStateResolver func(clusterName types.NamespacedName, esClient *esclient.Client) observer.State

	// resourcesStateResolver resolves the current state of the K8s resources from the K8s API
	resourcesStateResolver func(
		c client.Client,
		es v1alpha1.ElasticsearchCluster,
	) (*reconcile.ResourcesState, error)

	// clusterInitialMasterNodesEnforcer enforces that cluster.initial_master_nodes is set where relevant
	// this can safely be set to nil when it's not relevant (e.g for ES <= 6)
	clusterInitialMasterNodesEnforcer func(
		performableChanges mutation.PerformableChanges,
		resourcesState reconcile.ResourcesState,
	) (*mutation.PerformableChanges, error)

	// zen1SettingsUpdater updates the zen1 settings for the current pods.
	// this can safely be set to nil when it's not relevant (e.g when all nodes in the cluster is >= 7)
	zen1SettingsUpdater func(esClient *esclient.Client, allPods []corev1.Pod) error

	// zen2SettingsUpdater updates the zen2 settings for the current changes.
	// this can safely be set to nil when it's not relevant (e.g when all nodes in the cluster is <7)
	zen2SettingsUpdater func(
		esClient *esclient.Client,
		changes mutation.Changes,
		performableChanges mutation.PerformableChanges,
	) error

	// TODO: implement
	// // apiObjectsGarbageCollector garbage collects API objects for older versions once they are no longer needed.
	// apiObjectsGarbageCollector func(
	// 	c client.Client,
	// 	es v1alpha1.ElasticsearchCluster,
	// 	version version.Version,
	// 	state mutation.PodsState,
	// ) (reconcile.Result, error) // could get away with one impl
}

// Reconcile fulfills the Driver interface and reconciles the cluster resources.
func (d *defaultDriver) Reconcile(
	es v1alpha1.ElasticsearchCluster,
	reconcileState *reconcile.State,
) *reconcile.Results {
	results := &reconcile.Results{}

	genericResources, err := d.genericResourcesReconciler(d.Client, d.Scheme, es)
	if err != nil {
		return results.WithError(err)
	}

	if err := d.nodeCertificatesReconciler(
		d.Client,
		d.Scheme,
		d.ClusterCa,
		es,
		[]corev1.Service{genericResources.PublicService, genericResources.DiscoveryService},
	); err != nil {
		return results.WithError(err)
	}

	internalUsers, err := d.internalUsersReconciler(d.Client, d.Scheme, es)
	if err != nil {
		return results.WithError(err)
	}
	esClient := d.newElasticsearchClient(genericResources.PublicService, internalUsers.ControllerUser)

	observedState := d.observedStateResolver(k8s.ExtractNamespacedName(es.ObjectMeta), esClient)

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

	versionWideResources, err := d.versionWideResourcesReconciler(d.Client, d.Scheme, es, d.DynamicWatches)
	if err != nil {
		return results.WithError(err)
	}

	if err := snapshot.ReconcileSnapshotterCronJob(
		d.Client,
		d.Scheme,
		es,
		internalUsers.ControllerUser,
		d.OperatorImage,
	); err != nil {
		// it's ok to continue even if we cannot reconcile the cron job
		results.WithError(err)
	}

	changes, err := d.calculateChanges(versionWideResources, internalUsers, es, *resourcesState)
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
	performableChanges, err := mutation.CalculatePerformableChanges(es.Spec.UpdateStrategy, *changes, podsState)
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

	results.Apply(
		"reconcile-cluster-license",
		func() (controller.Result, error) {
			err := license.Reconcile(
				d.Client,
				d.DynamicWatches,
				es,
				esClient,
				observedState.ClusterLicense,
			)
			if err != nil && esReachable {
				reconcileState.AddEvent(
					corev1.EventTypeWarning,
					events.EventReasonUnexpected,
					fmt.Sprintf("Could not update cluster license: %s", err.Error()),
				)
				return defaultRequeue, err
			}
			return controller.Result{}, err
		},
	)

	if esReachable { // TODO this needs to happen outside of reconcileElasticsearchPods pending refactoring
		err = snapshot.EnsureSnapshotRepository(context.TODO(), esClient, es.Spec.SnapshotRepository)
		if err != nil {
			// TODO decide should this be a reason to stop this reconciliation loop?
			msg := "Could not ensure snapshot repository"
			reconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, msg)
			log.Error(err, msg)
		}
	}

	if d.clusterInitialMasterNodesEnforcer != nil {
		performableChanges, err = d.clusterInitialMasterNodesEnforcer(*performableChanges, *resourcesState)
		if err != nil {
			return results.WithError(err)
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

	if !esReachable {
		// We cannot manipulate ES allocation exclude settings if the ES cluster
		// cannot be reached, hence we cannot delete pods.
		// Probably it was just created and is not ready yet.
		// Let's retry in a while.
		log.Info("ES public service not ready yet for shard migration reconciliation. Requeuing.")

		reconcileState.UpdateElasticsearchPending(resourcesState.CurrentPods)

		return results.WithResult(defaultRequeue)
	}

	if d.zen2SettingsUpdater != nil {
		// TODO: would prefer to do this after MigrateData iff there's no changes? or is that an premature optimization?
		if err := d.zen2SettingsUpdater(
			esClient,
			*changes,
			*performableChanges,
		); err != nil {
			return results.WithResult(defaultRequeue).WithError(err)
		}
	}

	if !changes.HasChanges() {
		// Current state matches expected state

		// Update discovery for any previously created pods that have come up (see also below in create pod)
		if d.zen1SettingsUpdater != nil {
			if err := d.zen1SettingsUpdater(
				esClient,
				reconcile.AvailableElasticsearchNodes(resourcesState.CurrentPods),
			); err != nil {
				// TODO: reconsider whether this error should be propagated with results instead?
				log.Error(err, "Error during update discovery after having no changes, requeuing.")
				return results.WithResult(defaultRequeue)
			}
		}

		reconcileState.UpdateElasticsearchOperational(*resourcesState, observedState)
		return results
	}

	// Start migrating data away from all pods to be deleted
	leavingNodeNames := pod.PodListToNames(performableChanges.ToDelete)
	if err = migration.MigrateData(esClient, leavingNodeNames); err != nil {
		return results.WithError(errors.Wrap(err, "error during migrate data"))
	}

	newState := make([]corev1.Pod, len(resourcesState.CurrentPods))
	copy(newState, resourcesState.CurrentPods)

	// Shrink clusters by deleting deprecated pods
	for _, pod := range performableChanges.ToDelete {
		newState = removePodFromList(newState, pod)
		preDelete := func() error {
			if d.zen1SettingsUpdater != nil {
				if err := d.zen1SettingsUpdater(esClient, newState); err != nil {
					return err
				}
			}
			return nil
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
func removePodFromList(pods []corev1.Pod, pod corev1.Pod) []corev1.Pod {
	for i, p := range pods {
		if p.Name == pod.Name {
			return append(pods[:i], pods[i+1:]...)
		}
	}
	return pods
}

// calculateChanges calculates the changes we'd need to perform to go from the current cluster configuration to the
// desired one.
func (d *defaultDriver) calculateChanges(
	versionWideResources *VersionWideResources,
	internalUsers *user.InternalUsers,
	es v1alpha1.ElasticsearchCluster,
	resourcesState reconcile.ResourcesState,
) (*mutation.Changes, error) {
	expectedPodSpecCtxs, err := d.expectedPodsAndResourcesResolver(
		es,
		pod.NewPodSpecParams{
			ExtraFilesRef:     k8s.ExtractNamespacedName(versionWideResources.ExtraFilesSecret.ObjectMeta),
			KeystoreSecretRef: k8s.ExtractNamespacedName(versionWideResources.KeyStoreConfig.ObjectMeta),
			ProbeUser:         internalUsers.ControllerUser,
			ConfigMapVolume:   volume.NewConfigMapVolume(versionWideResources.GenericUnecryptedConfigurationFiles.Name, settings.ManagedConfigPath),
		},
		d.OperatorImage,
	)
	if err != nil {
		return nil, err
	}

	changes, err := mutation.CalculateChanges(
		expectedPodSpecCtxs,
		resourcesState,
		func(ctx pod.PodSpecContext) (corev1.Pod, error) {
			return esversion.NewPod(d.Version, es, ctx)
		},
	)
	if err != nil {
		return nil, err
	}
	return &changes, nil
}

// newElasticsearchClient creates a new Elasticsearch HTTP client for this cluster using the provided user
func (d *defaultDriver) newElasticsearchClient(service corev1.Service, user esclient.User) *esclient.Client {
	url := fmt.Sprintf("https://%s.%s.svc.cluster.local:%d", service.Name, service.Namespace, pod.HTTPPort)

	esClient := esclient.NewElasticsearchClient(
		d.Dialer, url, user, []*x509.Certificate{d.ClusterCa.Cert},
	)
	return esClient
}
