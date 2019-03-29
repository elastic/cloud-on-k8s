// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"context"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/restart"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/events"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/cleanup"
	esclient "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/license"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/migration"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/network"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/observer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/remotecluster"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/snapshot"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/user"
	esversion "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
		c k8s.Client,
		scheme *runtime.Scheme,
		es v1alpha1.Elasticsearch,
	) (*GenericResources, error)

	// nodeCertificatesReconciler reconciles node certificates
	nodeCertificatesReconciler func(
		c k8s.Client,
		scheme *runtime.Scheme,
		csrClient certificates.CSRClient,
		es v1alpha1.Elasticsearch,
		services []corev1.Service,
		trustRelationships []v1alpha1.TrustRelationship,
		caCertValidity time.Duration,
		caCertRotateBefore time.Duration,
		nodeCertValidity time.Duration,
		nodeCertRotateBefore time.Duration,
	) (*x509.Certificate, time.Time, error)

	// usersReconciler reconciles external and internal users and returns the current internal users.
	usersReconciler func(
		c k8s.Client,
		scheme *runtime.Scheme,
		es v1alpha1.Elasticsearch,
	) (*user.InternalUsers, error)

	// versionWideResourcesReconciler reconciles resources that may be specific to a version
	versionWideResourcesReconciler func(
		c k8s.Client,
		scheme *runtime.Scheme,
		es v1alpha1.Elasticsearch,
		trustRelationships []v1alpha1.TrustRelationship,
		w watches.DynamicWatches,
	) (*VersionWideResources, error)

	// expectedPodsAndResourcesResolver returns a list of pod specs with context that we would expect to find in the
	// Elasticsearch cluster.
	//
	// paramsTmpl argument is a partially filled NewPodSpecParams (TODO: refactor into its own params struct)
	expectedPodsAndResourcesResolver func(
		es v1alpha1.Elasticsearch,
		paramsTmpl pod.NewPodSpecParams,
		operatorImage string,
	) ([]pod.PodSpecContext, error)

	// observedStateResolver resolves the currently observed state of Elasticsearch from the ES API
	observedStateResolver func(clusterName types.NamespacedName, esClient esclient.Client) observer.State

	// resourcesStateResolver resolves the current state of the K8s resources from the K8s API
	resourcesStateResolver func(
		c k8s.Client,
		es v1alpha1.Elasticsearch,
	) (*reconcile.ResourcesState, error)

	// clusterInitialMasterNodesEnforcer enforces that cluster.initial_master_nodes is set where relevant
	// this can safely be set to nil when it's not relevant (e.g for ES <= 6)
	clusterInitialMasterNodesEnforcer func(
		performableChanges mutation.PerformableChanges,
		resourcesState reconcile.ResourcesState,
	) (*mutation.PerformableChanges, error)

	// zen1SettingsUpdater updates the zen1 settings for the current pods.
	// this can safely be set to nil when it's not relevant (e.g when all nodes in the cluster is >= 7)
	zen1SettingsUpdater func(esClient esclient.Client, allPods []corev1.Pod) error

	// zen2SettingsUpdater updates the zen2 settings for the current changes.
	// this can safely be set to nil when it's not relevant (e.g when all nodes in the cluster is <7)
	zen2SettingsUpdater func(
		esClient esclient.Client,
		changes mutation.Changes,
		performableChanges mutation.PerformableChanges,
	) error

	// TODO: implement
	// // apiObjectsGarbageCollector garbage collects API objects for older versions once they are no longer needed.
	// apiObjectsGarbageCollector func(
	// 	c k8s.Client,
	// 	es v1alpha1.Elasticsearch,
	// 	version version.Version,
	// 	state mutation.PodsState,
	// ) (reconcile.Result, error) // could get away with one impl
}

// Reconcile fulfills the Driver interface and reconciles the cluster resources.
func (d *defaultDriver) Reconcile(
	es v1alpha1.Elasticsearch,
	reconcileState *reconcile.State,
) *reconciler.Results {
	results := &reconciler.Results{}

	// garbage collect secrets attached to this cluster that we don't need anymore
	if err := cleanup.DeleteOrphanedSecrets(d.Client, es); err != nil {
		return results.WithError(err)
	}

	genericResources, err := d.genericResourcesReconciler(d.Client, d.Scheme, es)
	if err != nil {
		return results.WithError(err)
	}

	trustRelationships, err := nodecerts.LoadTrustRelationships(d.Client, es.Name, es.Namespace)
	if err != nil {
		return results.WithError(err)
	}

	caCert, caExpiration, err := d.nodeCertificatesReconciler(
		d.Client,
		d.Scheme,
		d.CSRClient,
		es,
		[]corev1.Service{genericResources.ExternalService, genericResources.DiscoveryService},
		trustRelationships,
		d.Parameters.CACertValidity,
		d.Parameters.CACertRotateBefore,
		d.Parameters.NodeCertValidity,
		d.Parameters.NodeCertRotateBefore,
	)
	if err != nil {
		return results.WithError(err)
	}
	// make sure to requeue before the CA cert expires
	results.WithResult(controller.Result{
		RequeueAfter: shouldRequeueIn(time.Now(), caExpiration, d.Parameters.CACertRotateBefore),
	})

	internalUsers, err := d.usersReconciler(d.Client, d.Scheme, es)
	if err != nil {
		return results.WithError(err)
	}

	resourcesState, err := d.resourcesStateResolver(d.Client, es)
	if err != nil {
		return results.WithError(err)
	}
	min, err := esversion.MinVersion(resourcesState.CurrentPods.Pods())
	if err != nil {
		return results.WithError(err)
	}
	if min == nil {
		min = &d.Version
	}

	esClient := d.newElasticsearchClient(
		protocolForESPods(resourcesState.CurrentPods),
		genericResources.ExternalService,
		internalUsers.ControllerUser,
		*min,
		caCert,
	)

	observedState := d.observedStateResolver(k8s.ExtractNamespacedName(&es), esClient)

	// always update the elasticsearch state bits
	if observedState.ClusterState != nil && observedState.ClusterHealth != nil {
		reconcileState.UpdateElasticsearchState(*resourcesState, observedState)
	}

	podsState := mutation.NewPodsState(*resourcesState, observedState)

	if err := d.supportedVersions.VerifySupportsExistingPods(resourcesState.CurrentPods.Pods()); err != nil {
		return results.WithError(err)
	}

	versionWideResources, err := d.versionWideResourcesReconciler(
		d.Client, d.Scheme, es, trustRelationships, d.DynamicWatches,
	)
	if err != nil {
		return results.WithError(err)
	}

	if err := snapshot.ReconcileSnapshotterCronJob(
		d.Client,
		d.Scheme,
		es,
		internalUsers.ControllerUser.Auth(),
		d.OperatorImage,
	); err != nil {
		// it's ok to continue even if we cannot reconcile the cron job
		results.WithError(err)
	}

	esReachable, err := services.IsServiceReady(d.Client, genericResources.ExternalService)
	if err != nil {
		return results.WithError(err)
	}

	if esReachable {
		err = snapshot.ReconcileSnapshotRepository(context.Background(), esClient, es.Spec.SnapshotRepository)
		if err != nil {
			msg := "Could not reconcile snapshot repository"
			reconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, msg)
			log.Error(err, msg)
			// requeue to retry but continue, as the failure might be caused by transient inconsistency between ES and
			// operator e.g. after certificates have been rotated
			results.WithResult(defaultRequeue)
		}

		err = remotecluster.UpdateRemoteCluster(d.Client, esClient, es, reconcileState)
		if err != nil {
			msg := "Could not update remote clusters in Elasticsearch settings"
			reconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, msg)
			log.Error(err, msg)
			results.WithResult(defaultRequeue)
		}
	}

	namespacedName := k8s.ExtractNamespacedName(&es)

	// There might be some ongoing creations and deletions our k8s client cache
	// hasn't seen yet. In such case, requeue until we are in-sync.
	// Otherwise, we could end up re-creating multiple times the same pod with
	// different generated names through multiple reconciliation iterations.
	if !d.PodsExpectations.Fulfilled(namespacedName) {
		log.Info("Pods creations and deletions expectations are not satisfied yet. Requeuing.")
		return results.WithResult(defaultRequeue)
	}

	changes, err := d.calculateChanges(versionWideResources, internalUsers, es, *resourcesState)
	if err != nil {
		return results.WithError(err)
	}

	log.Info(
		"Calculated all required changes",
		"to_create:", len(changes.ToCreate),
		"to_keep:", len(changes.ToKeep),
		"to_delete:", len(changes.ToDelete),
		"to_reuse", len(changes.ToReuse),
	)

	done, err := restart.HandlePodsReuse(d.Client, *changes)
	if err != nil {
		return results.WithError(err)
	}
	if !done {
		log.V(1).Info("Pods reuse is not over yet")
		return results.WithResult(defaultRequeue)
	}

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

	if d.clusterInitialMasterNodesEnforcer != nil {
		performableChanges, err = d.clusterInitialMasterNodesEnforcer(*performableChanges, *resourcesState)
		if err != nil {
			return results.WithError(err)
		}
	}

	for _, change := range performableChanges.ToCreate {
		d.PodsExpectations.ExpectCreation(namespacedName)
		if err := createElasticsearchPod(
			d.Client,
			d.Scheme,
			es,
			reconcileState,
			change.Pod,
			change.PodSpecCtx,
		); err != nil {
			// pod was not created, cancel our expectation by marking it observed
			d.PodsExpectations.CreationObserved(namespacedName)
			return results.WithError(err)
		}
	}
	// passed this point, any pods resource listing should check expectations first

	if !esReachable {
		// We cannot manipulate ES allocation exclude settings if the ES cluster
		// cannot be reached, hence we cannot delete pods.
		// Probably it was just created and is not ready yet.
		// Let's retry in a while.
		log.Info("ES external service not ready yet for shard migration reconciliation. Requeuing.")

		reconcileState.UpdateElasticsearchPending(resourcesState.CurrentPods.Pods())

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
				reconcile.AvailableElasticsearchNodes(resourcesState.CurrentPods.Pods()),
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
	leavingNodeNames := pod.PodListToNames(performableChanges.ToDelete.Pods())
	if err = migration.MigrateData(esClient, leavingNodeNames); err != nil {
		return results.WithError(errors.Wrap(err, "error during migrate data"))
	}

	// Shrink clusters by deleting deprecated pods
	if err = d.attemptPodsDeletion(
		performableChanges.ToDelete.Pods(),
		reconcileState,
		resourcesState,
		observedState,
		results,
		esClient,
		namespacedName,
	); err != nil {
		return results.WithError(err)
	}
	// past this point, any pods resource listing should check expectations first

	if changes.HasChanges() && !performableChanges.HasChanges() {
		// if there are changes we'd like to perform, but none that were performable, we try again later
		results.WithResult(defaultRequeue)
	}

	reconcileState.UpdateElasticsearchState(*resourcesState, observedState)

	return results
}

// attemptPodsDeletion deletes a list of pods after checking there is no migrating data for each of them
func (d *defaultDriver) attemptPodsDeletion(
	ToDelete []corev1.Pod,
	reconcileState *reconcile.State,
	resourcesState *reconcile.ResourcesState,
	observedState observer.State,
	results *reconciler.Results,
	esClient esclient.Client,
	namespacedName types.NamespacedName,
) error {
	newState := make([]corev1.Pod, len(resourcesState.CurrentPods))
	copy(newState, resourcesState.CurrentPods.Pods())
	for _, pod := range ToDelete {
		newState = removePodFromList(newState, pod)
		preDelete := func() error {
			if d.zen1SettingsUpdater != nil {
				if err := d.zen1SettingsUpdater(esClient, newState); err != nil {
					return err
				}
			}
			return nil
		}

		// do not delete a pod or expect a deletion if a data migration is in progress
		isMigratingData := migration.IsMigratingData(observedState, pod, ToDelete)
		if isMigratingData {
			log.Info("Skipping deletion because of migrating data", "pod", pod.Name)
			reconcileState.UpdateElasticsearchMigrating(*resourcesState, observedState)
			results.WithResult(defaultRequeue)
			continue
		}

		d.PodsExpectations.ExpectDeletion(namespacedName)
		result, err := deleteElasticsearchPod(
			d.Client,
			reconcileState,
			*resourcesState,
			pod,
			preDelete,
		)
		if err != nil {
			// pod was not deleted, cancel our expectation by marking it observed
			d.PodsExpectations.DeletionObserved(namespacedName)
			return err
		}
		results.WithResult(result)
	}
	return nil
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
	es v1alpha1.Elasticsearch,
	resourcesState reconcile.ResourcesState,
) (*mutation.Changes, error) {
	expectedPodSpecCtxs, err := d.expectedPodsAndResourcesResolver(
		es,
		pod.NewPodSpecParams{
			ExtraFilesRef:     k8s.ExtractNamespacedName(&versionWideResources.ExtraFilesSecret),
			KeystoreSecretRef: k8s.ExtractNamespacedName(&versionWideResources.KeyStoreConfig),
			ProbeUser:         internalUsers.ProbeUser.Auth(),
			ReloadCredsUser:   internalUsers.ReloadCredsUser.Auth(),
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
		mutation.ReuseOptions{
			ReusePods: es.Spec.UpdateStrategy.ReusePods,
			ReusePVCs: es.Spec.UpdateStrategy.ReusePersistentVolumes,
		},
	)
	if err != nil {
		return nil, err
	}
	return &changes, nil
}

// newElasticsearchClient creates a new Elasticsearch HTTP client for this cluster using the provided user
func (d *defaultDriver) newElasticsearchClient(protocol string, service corev1.Service, user user.User, v version.Version, caCert *x509.Certificate) esclient.Client {
	url := fmt.Sprintf("%s://%s.%s.svc.cluster.local:%d", protocol, service.Name, service.Namespace, network.HTTPPort)
	return esclient.NewElasticsearchClient(d.Dialer, url, user.Auth(), v, []*x509.Certificate{caCert})
}

// protocolForESPods inspects the given pods to return the protocol (http or https)
// that should be used to request the cluster.
// It does account for transient ongoing full cluster restarts where the cluster is
// migrating from TLS to non-TLS (or the other way around), during which we still need to
// reach the cluster with the protocol pods are using, not the "expected" one.
//
// Outside the reconciliation loop, it's usually sufficient to use `network.ProtocolForCluster()`
// to target the expected protocol (does not require the state of all pods).
// `network.ProtocolForCluster()` would not work during TLS to non-TLS migration,
// but a downtime is expected in such situation anyway.
func protocolForESPods(pods pod.PodsWithConfig) string {
	// default to https, unless at least one pod is configured for http
	for _, p := range pods {
		license := v1alpha1.LicenseType(p.Config[settings.XPackLicenseSelfGeneratedType])
		if network.ProtocolForLicense(license) == "http" {
			return "http"
		}
	}
	return "https"
}
