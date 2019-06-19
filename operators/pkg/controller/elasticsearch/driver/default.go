// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"crypto/x509"
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/cleanup"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/cluster"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/configmap"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/discovery"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pdb"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pvc"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/remotecluster"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/restart"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/user"
	esversion "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
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

	// usersReconciler reconciles external and internal users and returns the current internal users.
	usersReconciler func(
		c k8s.Client,
		scheme *runtime.Scheme,
		es v1alpha1.Elasticsearch,
	) (*user.InternalUsers, error)

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
	observedStateResolver func(clusterName types.NamespacedName, caCerts []*x509.Certificate, esClient esclient.Client) observer.State

	// resourcesStateResolver resolves the current state of the K8s resources from the K8s API
	resourcesStateResolver func(
		c k8s.Client,
		es v1alpha1.Elasticsearch,
	) (*reconcile.ResourcesState, error)

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

	if err := reconcileScriptsConfigMap(d.Client, d.Scheme, es); err != nil {
		return results.WithError(err)
	}

	genericResources, res := reconcileGenericResources(
		d.Client,
		d.Scheme,
		es,
	)
	if results.WithResults(res).HasError() {
		return results
	}

	certificateResources, res := certificates.Reconcile(
		d.Client,
		d.Scheme,
		d.DynamicWatches,
		es,
		[]corev1.Service{genericResources.ExternalService},
		d.Parameters.CACertValidity,
		d.Parameters.CACertRotateBefore,
		d.Parameters.CertValidity,
		d.Parameters.CertRotateBefore,
	)
	if results.WithResults(res).HasError() {
		return results
	}

	if err := settings.ReconcileSecureSettings(d.Client, reconcileState.Recorder, d.Scheme, d.DynamicWatches, es); err != nil {
		return results.WithError(err)
	}

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

	observedState := d.observedStateResolver(
		k8s.ExtractNamespacedName(&es),
		certificateResources.TrustedHTTPCertificates,
		d.newElasticsearchClient(
			genericResources.ExternalService,
			internalUsers.ControllerUser,
			*min,
			certificateResources.TrustedHTTPCertificates,
		))

	// always update the elasticsearch state bits
	if observedState.ClusterState != nil && observedState.ClusterHealth != nil {
		reconcileState.UpdateElasticsearchState(*resourcesState, observedState)
	}

	if err := pdb.Reconcile(d.Client, d.Scheme, es); err != nil {
		return results.WithError(err)
	}

	podsState := mutation.NewPodsState(*resourcesState, observedState)

	if err := d.supportedVersions.VerifySupportsExistingPods(resourcesState.CurrentPods.Pods()); err != nil {
		return results.WithError(err)
	}

	// TODO: support user-supplied certificate (non-ca)
	esClient := d.newElasticsearchClient(
		genericResources.ExternalService,
		internalUsers.ControllerUser,
		*min,
		certificateResources.TrustedHTTPCertificates,
	)
	defer esClient.Close()

	esReachable, err := services.IsServiceReady(d.Client, genericResources.ExternalService)
	if err != nil {
		return results.WithError(err)
	}

	if esReachable {
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

	changes, err := d.calculateChanges(internalUsers, es, *resourcesState)
	if err != nil {
		return results.WithError(err)
	}

	log.Info(
		"Calculated all required changes",
		"to_create:", len(changes.ToCreate),
		"to_keep:", len(changes.ToKeep),
		"to_delete:", len(changes.ToDelete),
	)

	// restart ES processes that need to be restarted before going on with other changes
	done, err := restart.HandleESRestarts(
		restart.RestartContext{
			Cluster:        es,
			EventsRecorder: reconcileState.Recorder,
			K8sClient:      d.Client,
			Changes:        *changes,
			Dialer:         d.Dialer,
			EsClient:       esClient,
		},
	)
	if err != nil {
		return results.WithError(err)
	}
	if !done {
		log.V(1).Info("Pods restart is not over yet, re-queueing.")
		return results.WithResult(defaultRequeue)
	}

	// figure out what changes we can perform right now
	performableChanges, err := mutation.CalculatePerformableChanges(
		es.Spec.UpdateStrategy,
		*changes,
		// copy the pods state because this method is destructive
		podsState.Copy(),
	)
	if err != nil {
		return results.WithError(err)
	}

	// zen1 has certain limits on what changes can be safely performed concurrently.
	if err := discovery.ApplyZen1Limitations(d.Client, podsState, performableChanges, esReachable); err != nil {
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

	// bootstrap a Zen2 cluster if required
	if err := discovery.Zen2InjectInitialMasterNodesIfBootstrapping(performableChanges, *resourcesState); err != nil {
		return results.WithError(err)
	}

	// Compute seed hosts based on current masters with a podIP
	if err := discovery.UpdateSeedHostsConfigMap(d.Client, d.Scheme, es, resourcesState.AllPods); err != nil {
		return results.WithError(err)
	}

	logicalCluster := cluster.NewDirectCluster(esClient, observedState)

	if err := logicalCluster.PrepareForDeletionPods(d.Client, es, podsState, performableChanges); err != nil {
		return results.WithError(err)
	}

	if err := logicalCluster.FilterDeletablePods(d.Client, es, podsState, performableChanges); err != nil {
		return results.WithError(err)
	}

	if esReachable {
		// TODO: update the following comments to be a little more applicable
		// Call Zen1 setting updater before new masters are created to ensure that they immediately start with the
		// correct value for minimum_master_nodes.
		// For instance if a 3 master nodes cluster is updated and a grow-and-shrink strategy of one node is applied then
		// minimum_master_nodes is increased from 2 to 3 for new and current nodes.

		// update the cluster for the infrastructure changes we're about to perform
		if err := logicalCluster.OnInfrastructureState(
			d.Client, es, podsState, performableChanges, reconcileState,
		); err != nil {
			return results.WithError(err)
		}

		// update zen 1 configuration files on disk in preparation for the changes we're about to perform
		if err := discovery.Zen1UpdateMinimumMasterNodesConfig(d.Client, es, podsState, performableChanges); err != nil {
			return results.WithError(err)
		}
	}

	// List the orphaned PVCs before the Pods are created.
	// If there are some orphaned PVCs they will be adopted and remove sequentially from the list when Pods are created.
	orphanedPVCs, err := pvc.FindOrphanedVolumeClaims(d.Client, es)
	if err != nil {
		return results.WithError(err)
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
			orphanedPVCs,
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

	if !changes.HasChanges() {
		// Current state matches expected state
		reconcileState.UpdateElasticsearchOperational(*resourcesState, observedState)
		return results
	}

	// Shrink clusters by deleting deprecated pods
	if err = d.attemptPodsDeletion(
		performableChanges.ToDelete.Pods(),
		reconcileState,
		resourcesState,
		observedState,
		results,
		esClient,
		es,
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
	pods []corev1.Pod,
	reconcileState *reconcile.State,
	resourcesState *reconcile.ResourcesState,
	observedState observer.State,
	results *reconciler.Results,
	esClient esclient.Client,
	elasticsearch v1alpha1.Elasticsearch,
) error {
	for _, p := range pods {
		namespacedName := k8s.ExtractNamespacedName(&elasticsearch)
		d.PodsExpectations.ExpectDeletion(namespacedName)
		result, err := deleteElasticsearchPod(
			d.Client,
			reconcileState,
			*resourcesState,
			p,
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

// calculateChanges calculates the changes we'd need to perform to go from the current cluster configuration to the
// desired one.
func (d *defaultDriver) calculateChanges(
	internalUsers *user.InternalUsers,
	es v1alpha1.Elasticsearch,
	resourcesState reconcile.ResourcesState,
) (*mutation.Changes, error) {
	expectedPodSpecCtxs, err := d.expectedPodsAndResourcesResolver(
		es,
		pod.NewPodSpecParams{
			ProbeUser:    internalUsers.ProbeUser.Auth(),
			KeystoreUser: internalUsers.KeystoreUser.Auth(),
			UnicastHostsVolume: volume.NewConfigMapVolume(
				name.UnicastHostsConfigMap(es.Name), esvolume.UnicastHostsVolumeName, esvolume.UnicastHostsVolumeMountPath,
			),
		},
		d.OperatorImage,
	)
	if err != nil {
		return nil, err
	}

	changes, err := mutation.CalculateChanges(
		es,
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
func (d *defaultDriver) newElasticsearchClient(service corev1.Service, user user.User, v version.Version, caCerts []*x509.Certificate) esclient.Client {
	url := fmt.Sprintf("https://%s.%s.svc:%d", service.Name, service.Namespace, network.HTTPPort)
	return esclient.NewElasticsearchClient(d.Dialer, url, user.Auth(), v, caCerts)
}

func reconcileScriptsConfigMap(c k8s.Client, scheme *runtime.Scheme, es v1alpha1.Elasticsearch) error {
	fsScript, err := initcontainer.RenderPrepareFsScript()
	if err != nil {
		return err
	}

	scriptsConfigMap := configmap.NewConfigMapWithData(
		types.NamespacedName{Namespace: es.Namespace, Name: name.ScriptsConfigMap(es.Name)},
		map[string]string{
			pod.ReadinessProbeScriptConfigKey:      pod.ReadinessProbeScript,
			initcontainer.PrepareFsScriptConfigKey: fsScript,
		})

	if err := configmap.ReconcileConfigMap(c, scheme, es, scriptsConfigMap); err != nil {
		return err
	}

	return nil
}
